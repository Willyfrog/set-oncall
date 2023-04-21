package main

import (
	"context"
	"encoding/json"
	"fmt"
	"flag"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"
	"github.com/opsgenie/opsgenie-go-sdk-v2/client"
	"github.com/opsgenie/opsgenie-go-sdk-v2/schedule"
	log "github.com/sirupsen/logrus"
)

// ScheduleMap is the format within config to write
// "opsgenie schedule name": "display name"
type ScheduleMap map[string]string

// Config type defilnes the json format for the configuration file
//   {
//     "schedules": {
//       "opsgenie": "The OpsGenie Schedule",
//     }
//   }
type Config struct {
	Schedules ScheduleMap `json:"schedules"`
}

// Results schedule:[list of users on rotation]
type Results map[string][]string

// UserSet opsgenieEmail: MMusername
type UserSet map[string]string


func readConfigSchedules(filepath string) (ScheduleMap, error) {
	// Read the contents of the config file into a byte slice
	fileContents, err := ioutil.ReadFile(filepath)
	if err != nil {
		return nil, err
	}

	// Unmarshal the JSON contents of the file into a Config struct
	var config Config
	if err := json.Unmarshal(fileContents, &config); err != nil {
		return nil, err
	}

	return config.Schedules, nil
}


const earlyShiftFormat = "%sT09:00:00+00:00"
const lateShiftFormat = "%sT17:00:00+00:00"

func getShiftDate(thisWeek bool, early bool) *time.Time {
	day := time.Now()
	if !thisWeek {
		day = day.AddDate(0, 0, 7-int(day.Weekday())+1)
	}

	hours := 9
	if !early {
		hours = 17
	}
	shift, _ := time.ParseDuration(fmt.Sprintf("%vh", hours-day.Hour()))
	day = day.Add(shift)
	return &day
}

func getScheduleParticipants(scheduleName string, thisWeek bool, client *schedule.Client) (UserSet, error) {
	
	flat := true
	earlyReq := &schedule.GetOnCallsRequest{
		Flat: &flat,
		Date: getShiftDate(thisWeek, true),
		ScheduleIdentifierType: schedule.Name,
		ScheduleIdentifier: scheduleName,
	}
	lateReq := &schedule.GetOnCallsRequest{
		Flat: &flat,
		Date: getShiftDate(thisWeek, false),
		ScheduleIdentifierType: schedule.Name,
		ScheduleIdentifier: scheduleName,
	}

	earlyOnCall, err := client.GetOnCalls(context.TODO(), earlyReq)
	if err != nil {
		log.Fatal("Error trying to get the early shift")
		return nil, err
	}
	lateOnCall, err := client.GetOnCalls(context.TODO(), lateReq)
	if err != nil {
		log.Fatal("Error trying to get the late shift")
		return nil, err
	}
	results := UserSet{} // Using a map to prevent duplicates
	for _, user := range(earlyOnCall.OnCallRecipients) {
		results[user] = "" //will add MM username here
	}
	for _, user := range(lateOnCall.OnCallRecipients) {
		results[user] = "" //will add MM username here
	}

	return results, nil
}

func main() {
	apiKey := os.Getenv("OPSGENIE_API_KEY")
	if apiKey == "" {
		fmt.Println("OPSGENIE_API_KEY environment variable not set.")
		return
	}
	webhookURL := os.Getenv("MATTERMOST_WEBHOOK_URL")
	if webhookURL == "" {
		log.Fatal("MATTERMOST_WEBHOOK_URL environment variable not set.")
		return
	}

	// Define command line flags
	nextWeek := flag.Bool("next-week", false, "Query users who will be on-call next week, set to false (default) for this week")
	flag.Parse()
	thisWeek := !*nextWeek

	schedules, err := readConfigSchedules("./config.json")
	if err != nil {
		fmt.Println("Error reading the schedules", err)
	}

	scheduleClient, err := schedule.NewClient(&client.Config{
		ApiKey: apiKey,
	})
	if err != nil {
		log.Fatal("Not able to create an OpsGenie client")
	}
	
	var results = make(Results)
	for scheduleName, scheduleDisplay := range schedules {
		onCallResponse, err := getScheduleParticipants(scheduleName, thisWeek, scheduleClient)
		if err != nil {
			fmt.Println("Error getting the schedule ", scheduleDisplay, err)
		}
		results[scheduleDisplay] = []string{}
		for onCallmail := range onCallResponse {
			// todo: translate into MM usernames
			results[scheduleDisplay] = append(results[scheduleDisplay], onCallmail)
		}
	}

	var message string
	if thisWeek {
		message = "The following people are currently on call this week:\n"
	} else {
		message = "Heads up for next week on call rotation:\n"
	}
	for scheduleName, usernames := range results {
		message = message + fmt.Sprintf(" - %s: %s\n", scheduleName, strings.Join(usernames, ", "))
	}
	payload := map[string]string{
		"text": message,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		fmt.Println("Error encoding payload:", err)
		return
	}
	resp, err := http.Post(webhookURL, "application/json", strings.NewReader(string(payloadBytes)))
	if err != nil {
		fmt.Println("Error sending webhook:", err)
		return
	}
	defer resp.Body.Close()

	fmt.Println("Successfully sent on-call users to Mattermost channel.")
}
