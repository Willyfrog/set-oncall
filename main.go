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
	model "github.com/mattermost/mattermost-server/v6/model"
)

// ScheduleMap is the format within config to write
// "opsgenie schedule name": "display name"
type ScheduleMap map[string]string

// TitleLink consist on a list of two title links to put on top of the card, the first one is for the current week and the second one is for the incoming week
type TitleLink []string

// Titles consist on a list of two titles texts to put on top of the card, the first one is for the current week and the second one is for the incoming week
type Titles []string

// Config type defilnes the json format for the configuration file
//   {
//     "schedules": {
//       "opsgenie": "The OpsGenie Schedule",
//     }
//     "title": ["this week's title", "next week one"],
//     "titleLink": ["link on title for this week"],
//     "username": "bot",
//     "iconurl": "some image to display"
//   }
type Config struct {
	Schedules ScheduleMap `json:"schedules"`
	Titles Titles `json:"title"`
	TitleLinks TitleLink `json:"titleLink"`
	Username string `json:"username"`
	IconURL string `json:"iconurl"`
	SiteURL string `json:"siteurl"`
}

// Results schedule:[list of users on rotation]
type Results map[string][]string

// UserSet opsgenieEmail: MMusername
type UserSet map[string]string


func readConfig(filepath string) (*Config, error) {
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

	return &config, nil
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

func getUserFromMail(email string, client *model.Client4) (string, error) {
	log.Print("Converting %s into a mm user: ", email)
	user, _, err := client.GetUserByEmail(email, "")
	if err != nil {
		log.Error(err)
		return "", err
	}
	log.Print("MM user is: @", user.Username)
	return fmt.Sprintf("@%s", user.Username), err
}

func getMMUsers(users []string, client *model.Client4) ([]string, error) {
	var mmusers []string
	numErrors := 0
	for _, email := range(users) {
		username, err := getUserFromMail(email, client)
		if err != nil {
			username = email
			numErrors++
		}
		mmusers = append(mmusers, username)
	}
	var finalError error
	finalError = nil
	if numErrors == len(users) {
		finalError = fmt.Errorf("Too many errors connecting to Mattermost instance %s to get the usernames", client.APIURL)
	}
	return mmusers, finalError
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
	mattermostKey := os.Getenv("MATTERMOST_API_KEY")
	if apiKey == "" {
		fmt.Println("MATTERMOST_API_KEY environment variable not set.")
		return
	}

	// Define command line flags
	nextWeek := flag.Bool("next-week", false, "Query users who will be on-call next week, set to false (default) for this week")
	flag.Parse()
	thisWeek := !*nextWeek

	config, err := readConfig("./config.json")
	if err != nil {
		fmt.Println("Error reading config.json", err)
	}
	// check config
	if config.SiteURL == "" {
		log.Fatal("Site Url not set in the config")
	}

	scheduleClient, err := schedule.NewClient(&client.Config{
		ApiKey: apiKey,
	})
	if err != nil {
		log.Fatal("Not able to create an OpsGenie client")
	}
	mmClient := model.NewAPIv4Client(config.SiteURL)
	mmClient.SetToken(mattermostKey)
	
	var results = make(Results)
	for scheduleName, scheduleDisplay := range config.Schedules {
		onCallResponse, err := getScheduleParticipants(scheduleName, thisWeek, scheduleClient)
		if err != nil {
			fmt.Println("Error getting the schedule ", scheduleDisplay, err)
		}
		results[scheduleDisplay] = []string{}
		for onCallmail := range onCallResponse {
			results[scheduleDisplay] = append(results[scheduleDisplay], onCallmail)
		}
		mmUsers, err := getMMUsers(results[scheduleDisplay], mmClient)
		if err != nil {
			log.Fatal(err)
		}
		results[scheduleDisplay] = mmUsers
	}

	var title string
	var titleLink string
	if thisWeek {
		if len(config.Titles) >= 1 {
			title = config.Titles[0]
		} else {
			title = ":rotating_light: Who is on Call this week :rotating_light:"
		}
		if len(config.TitleLinks) >= 1 {	
			titleLink = config.TitleLinks[0]
		} else {
			titleLink = "https://www.mattermost.com"
		}
	} else {
		if len(config.Titles) > 1 {
			title = config.Titles[1]
		} else {
			title = "Heads up for next week on call rotation:"
		}
		if len(config.TitleLinks) > 1 {	
			titleLink = config.TitleLinks[1]
		} else {
			titleLink = "https://www.mattermost.com"
		}
	}
	fields := []*model.SlackAttachmentField{}
	for scheduleName, usernames := range results {
		fields = append(fields, &model.SlackAttachmentField{Title: scheduleName, Value: strings.Join(usernames, ", "), Short: true})
	}

	attachment := &model.SlackAttachment{
		Color:     "#ff0000",
		Fields: fields,
	}

	
	payload := model.CommandResponse{
		Username:    "SET Team little helper",
		IconURL:     "https://upload.wikimedia.org/wikipedia/commons/0/01/Creative-Tail-People-superman.svg",
		Text:        fmt.Sprintf("### [%s](%s)", title, titleLink),
		Attachments: []*model.SlackAttachment{attachment},
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
