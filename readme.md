The Opsgenie API key, schedule name, and Mattermost webhook URL are read from environment variables.
To set these environment variables, you can use the export command in your terminal:

```sh
export OPSGENIE_API_KEY=<your_api_key>
export MATTERMOST_WEBHOOK_URL=<your_webhook_url>
export MATTERMOST_API_KEY=<your_api_key>
```

After that, you can run the project by issuing

```sh
go run main.go
```
or
```sh
go run main.go -next-week
```
