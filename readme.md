The Opsgenie API key, schedule name, and Mattermost webhook URL are read from environment variables.
To set these environment variables, you can use the export command in your terminal:

```sh
export OPSGENIE_API_KEY=<your_api_key>
export OPSGENIE_SCHEDULE_NAME=<your_schedule_name>
export MATTERMOST_WEBHOOK_URL=<your_webhook_url>
```

After that, you can run the project by issuing

```sh
go run main.go
```
