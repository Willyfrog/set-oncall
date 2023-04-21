package main

import (
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
	"github.com/opsgenie/opsgenie-go-sdk-v2/user"
)

type OnCallResponse struct {
	Data struct {
		OnCallParticipants []string `json:"onCallRecipients"`
	} `json:"data"`
}

type ScheduleMap map[string]string

type Results map[string][]string

type Config struct {
	Schedules ScheduleMap `json:"schedules"`
}

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

func nextMonday() {
}

func getScheduleParticipants(scheduleName string, thisWeek bool, apiKey string) (*OnCallResponse, error) {
	var url string
	if thisWeek {
		url = fmt.Sprintf("https://api.opsgenie.com/v2/schedules/%s/on-calls?scheduleIdentifierType=name&flat=true", scheduleName)
	} else {
		now := time.Now()
		startOfWeek := now.AddDate(0, 0, -int(now.Weekday())+1)
		endOfWeek := startOfWeek.AddDate(0, 0, 7)
		url = fmt.Sprintf("https://api.opsgenie.com/v2/schedules/%s/on-calls?startDate=%s&endDate=%s&scheduleIdentifierType=name&flat=true", scheduleName, startOfWeek.Format("2006-01-02"), endOfWeek.Format("2006-01-02"))
	}
	fmt.Println("Accessing url ", url)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Println("Error creating request:", err)
		return nil, err
	}
	req.Header.Set("Authorization", "GenieKey "+apiKey)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error making request:", err)
		return nil, err
	}
	defer resp.Body.Close()
	fmt.Println("Opsgenie status response:", resp.Status)
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error reading response:", err)
		return nil, err
	}

	var onCallResponse OnCallResponse
	err = json.Unmarshal(body, &onCallResponse)
	if err != nil {
		fmt.Println("Error decoding response:", err)
		return nil, err
	}
	return &onCallResponse, nil
}

func main() {
	apiKey := os.Getenv("OPSGENIE_API_KEY")
	if apiKey == "" {
		fmt.Println("OPSGENIE_API_KEY environment variable not set.")
		return
	}
	webhookURL := os.Getenv("MATTERMOST_WEBHOOK_URL")
	if webhookURL == "" {
		fmt.Println("MATTERMOST_WEBHOOK_URL environment variable not set.")
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
	var results = make(Results)
	for scheduleName, scheduleDisplay := range schedules {
		onCallResponse, err := getScheduleParticipants(scheduleName, thisWeek, apiKey)
		if err != nil {
			fmt.Println("Error getting the schedule ", scheduleDisplay, err)
		}
		results[scheduleDisplay] = []string{}
		for _, onCall := range onCallResponse.Data.OnCallParticipants {
			// todo: translate into MM usernames
			results[scheduleDisplay] = append(results[scheduleDisplay], onCall)
		}
	}
	
	for scheduleName, usernames := range results {
		var messageTemplate string
		if thisWeek {
			messageTemplate = "The following users are currently on call for schedule %s: %s"
		} else {
			messageTemplate = "The following users will be on call for schedule %s next week: %s"
		}
		message := fmt.Sprintf(messageTemplate, scheduleName, strings.Join(usernames, ", "))
		
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
	}

	fmt.Println("Successfully sent on-call users to Mattermost channel.")
}
