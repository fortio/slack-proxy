# Slack Message Proxy

This tool serves as a proxy for applications aiming to dispatch messages to Slack *nicely*. While many applications adopt a "fire and forget" approach for notifications, this method remains effective only up to a certain scale. When operating at larger scales, there's a risk of reaching Slack's rate limits. It's advisable to maintain a rate of one message per second (as per Slack documentation). Slack does provide a burst capacity. However, the specifics of this burst capacity remain undetermined.

## Use-case

For those managing multiple applications, a practical approach is to create a separate Slack app or webhook for each application. That method is prefered. **However**, if you are operating a singular system, like Alertmanager or Grafana, and wish to avoid the complexities of providing individual teams with:

- Their distinct webhooks
- Unique notification channel keys
- Possible additional routing configurations

Then, this tool is your ideal solution.

## What this tool tries to solve

Many applications posting messages to Slack either overlook Slack's rate limits or neglect to process Slack's response. By incorporating a rate limiter and a queue, we can not only ensure compliance with Slack's guidelines but also enhance the likelihood of our messages being successfully delivered.

By being a 1:1 forwarding proxy, you simply POST to this application instead, and it will get forwarded to Slack.

Furthermore, by adding observability, we can have a much clearer picture of:
- Requests per second
- To which channel?
- Are there failures and at what rate?

These type of insights are currently not possible to know via Slack, and only via different methods if your applications are instrumented that way (which they often aren't)


## ToDo's

- Currently, we do not use the original header bearer token. It is required you setup this application with a slack webhook. I personally think that's fine/good. Open for suggestions..
- Implement our own bearer token auth method. This way the application can run protected (at the moment anyone can POST against this application and it will send the message to Slack) - open for other suggestions..
- Build + Docker image
- Code check
- Actually check Slack errors if they are retryable or not
- How to run multiple replicas with each their own API key?
- Log user errors (i.e. non-retryable errors)
- Check if "channel does not exist" error is present, then ignore any message to said channel for a period of time to ease requests towards Slack

## Done ToDo's

- Implement observability, we want to have statistics for:
    - Requests/s per channel
    - Failed requests due to user error
    - Queue size


## Slack Application manifest

This manifest is required when making an application that can:
- Use a single token
- Post to any (public) channel
- Change it's name

```yaml
display_information:
  name: your-name
features:
  bot_user:
    display_name: your-default-display-name
    always_online: false
oauth_config:
  scopes:
    user:
      - chat:write
    bot:
      - chat:write
      - chat:write.customize
      - chat:write.public
settings:
  org_deploy_enabled: false
  socket_mode_enabled: false
  token_rotation_enabled: false

```

# Flags

### Required

- `--token` : Bearer token for the Slack API. 
   - Example: `--token=YOUR_BEARER_TOKEN`

 ### Optional

> I would recommend not altering these values until you have a good understanding how it performs for your workload

- `--maxRetries` : Maximum number of retries for posting a message.
   - Default: *`3`*
   - Example: `--maxRetries=5`

- `--initialBackoffMs` : Initial backoff in milliseconds for retries.
   - Default: *`1000`*
   - Example: `--initialBackoffMs=2000`

- `--slackURL` : Slack Post Message API URL.
   - Default: *`https://slack.com/api/chat.postMessage`*
   - Example: `--slackURL=https://api.slack.com/your-endpoint`

- `--queueSize` : Maximum number of messages in the queue.
   - Default: *`100`*
   - Example: `--queueSize=200`

- `--burst` : Maximum number of burst messages to allow.
   - Default: *`3`*
   - Example: `--burst=2`

- `--metricsPort` : Port used for the `/metrics` endpoint
    - Default: *`:9090`*
    - Example: `--metricsPort :9090`

- `--applicationPort` : Port used for the application endpoint (where you send your requests to)
    - Default: *`:8080`*
    - Example: `--applicationPort :8080`    