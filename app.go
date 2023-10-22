// app.go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"time"

	"fortio.org/log"
	"golang.org/x/time/rate"
)

type SlackMessenger interface {
	PostMessage(req SlackPostMessageRequest, url string, token string) error
}

type SlackClient struct {
	client *http.Client
}

var slackPermanentErrors = map[string]string{
	"as_user_not_supported":                    "The as_user parameter does not function with workspace apps.",
	"channel_not_found":                        "Value passed for channel was invalid.",
	"duplicate_channel_not_found":              "Channel associated with client_msg_id was invalid.",
	"duplicate_message_not_found":              "No duplicate message exists associated with client_msg_id.",
	"ekm_access_denied":                        "Administrators have suspended the ability to post a message.",
	"invalid_blocks":                           "Blocks submitted with this message are not valid",
	"invalid_blocks_format":                    "The blocks is not a valid JSON object or doesn't match the Block Kit syntax.",
	"invalid_metadata_format":                  "Invalid metadata format provided",
	"invalid_metadata_schema":                  "Invalid metadata schema provided",
	"is_archived":                              "Channel has been archived.",
	"messages_tab_disabled":                    "Messages tab for the app is disabled.",
	"metadata_must_be_sent_from_app":           "Message metadata can only be posted or updated using an app token",
	"metadata_too_large":                       "Metadata exceeds size limit",
	"msg_too_long":                             "Message text is too long",
	"no_text":                                  "No message text provided",
	"not_in_channel":                           "Cannot post user messages to a channel they are not in.",
	"restricted_action":                        "A workspace preference prevents the authenticated user from posting.",
	"restricted_action_non_threadable_channel": "Cannot post thread replies into a non_threadable channel.",
	"restricted_action_read_only_channel":      "Cannot post any message into a read-only channel.",
	"restricted_action_thread_locked":          "Cannot post replies to a thread that has been locked by admins.",
	"restricted_action_thread_only_channel":    "Cannot post top-level messages into a thread-only channel.",
	"slack_connect_canvas_sharing_blocked":     "Admin has disabled Canvas File sharing in all Slack Connect communications",
	"slack_connect_file_link_sharing_blocked":  "Admin has disabled Slack File sharing in all Slack Connect communications",
	"team_access_not_granted":                  "The token used is not granted the specific workspace access required to complete this request.",
	"too_many_attachments":                     "Too many attachments were provided with this message. A maximum of 100 attachments are allowed on a message.",
	"too_many_contact_cards":                   "Too many contact_cards were provided with this message. A maximum of 10 contact cards are allowed on a message.",
	"cannot_reply_to_message":                  "This message type cannot have thread replies.",
	"access_denied":                            "Access to a resource specified in the request is denied.",
	"account_inactive":                         "Authentication token is for a deleted user or workspace when using a bot token.",
	"deprecated_endpoint":                      "The endpoint has been deprecated.",
	"enterprise_is_restricted":                 "The method cannot be called from an Enterprise.",
	"invalid_auth":                             "Some aspect of authentication cannot be validated. Either the provided token is invalid or the request originates from an IP address disallowed from making the request.",
	"method_deprecated":                        "The method has been deprecated.",
	"missing_scope":                            "The token used is not granted the specific scope permissions required to complete this request.",
	"not_allowed_token_type":                   "The token type used in this request is not allowed.",
	"not_authed":                               "No authentication token provided.",
	"no_permission":                            "The workspace token used in this request does not have the permissions necessary to complete the request. Make sure your app is a member of the conversation it's attempting to post a message to.",
	"org_login_required":                       "The workspace is undergoing an enterprise migration and will not be available until migration is complete.",
	"token_expired":                            "Authentication token has expired",
	"token_revoked":                            "Authentication token is for a deleted user or workspace or the app has been removed when using a user token.",
	"two_factor_setup_required":                "Two factor setup is required.",
	"accesslimited":                            "Access to this method is limited on the current network",
	"fatal_error":                              "The server could not complete your operation(s) without encountering a catastrophic error. It's possible some aspect of the operation succeeded before the error was raised.",
	"internal_error":                           "The server could not complete your operation(s) without encountering an error, likely due to a transient issue on our end. It's possible some aspect of the operation succeeded before the error was raised.",
	"invalid_arg_name":                         "The method was passed an argument whose name falls outside the bounds of accepted or expected values. This includes very long names and names with non-alphanumeric characters other than _. If you get this error, it is typically an indication that you have made a very malformed API call.",
	"invalid_arguments":                        "The method was either called with invalid arguments or some detail about the arguments passed is invalid, which is more likely when using complex arguments like blocks or attachments.",
	"invalid_array_arg":                        "The method was passed an array as an argument. Please only input valid strings.",
	"invalid_charset":                          "The method was called via a POST request, but the charset specified in the Content-Type header was invalid. Valid charset names are: utf-8 iso-8859-1.",
	"invalid_form_data":                        "The method was called via a POST request with Content-Type application/x-www-form-urlencoded or multipart/form-data, but the form data was either missing or syntactically invalid.",
	"invalid_post_type":                        "The method was called via a POST request, but the specified Content-Type was invalid. Valid types are: application/json application/x-www-form-urlencoded multipart/form-data text/plain.",
	"missing_post_type":                        "The method was called via a POST request and included a data payload, but the request did not include a Content-Type header.",
	"ratelimited":                              "The request has been ratelimited. Refer to the Retry-After header for when to retry the request.",
	"service_unavailable":                      "The service is temporarily unavailable",
	"team_added_to_org":                        "The workspace associated with your request is currently undergoing migration to an Enterprise Organization. Web API and other platform operations will be intermittently unavailable until the transition is complete.",
}

var slackRetryErrors = map[string]string{
	"message_limit_exceeded": "Members on this team are sending too many messages. For more details, see https://slack.com/help/articles/115002422943-Usage-limits-for-free-workspaces",
	"rate_limited":           "Application has posted too many messages, read the Rate Limit documentation for more information",
	"fatal_error":            "The server could not complete your operation(s) without encountering a catastrophic error. It's possible some aspect of the operation succeeded before the error was raised.",
	"internal_error":         "The server could not complete your operation(s) without encountering an error, likely due to a transient issue on our end. It's possible some aspect of the operation succeeded before the error was raised.",
	"ratelimited":            "The request has been ratelimited. Refer to the Retry-After header for when to retry the request.",
	"request_timeout":        "The method was called via a POST request, but the POST data was either missing or truncated.",
}

var doNotProcessChannels = map[string]time.Time{}

func CheckError(err string, channel string) (retryable bool, pause bool, description string) {
	// Special case for channel_not_found, we don't want to retry this one right away.
	// We are making it a 'soft failure' so that we don't keep retrying it for a period of time for any message that is sent to a channel that doesn't exist.
	// We keep track of said channel in a map, and we will retry it after a period of time.
	if err == "channel_not_found" {
		doNotProcessChannels[channel] = time.Now()
		return true, true, "Channel not found"
	}

	description, exists := slackRetryErrors[err]
	if exists {
		return true, false, description
	}

	description, exists = slackPermanentErrors[err]
	if exists {
		return false, false, description
	}

	// This should not happen, but if it does, we just try to retry it
	return true, false, "Unknown error"
}

func (s *SlackClient) PostMessage(request SlackPostMessageRequest, url string, token string) error {
	jsonValue, err := json.Marshal(request)
	if err != nil {
		return err
	}
	// Detach from the caller/new context. TODO: have some timeout (or use jrpc package functions which do that already)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewBuffer(jsonValue))
	if err != nil {
		return err
	}

	// Charset is required to remove warnings from Slack. Maybe it's nice to have it configurable. /shrug
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	// Documentation says that you are allowed the POST the token instead, however that does simply not work. Hence why we are using the Authorization header.
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var slackResp SlackResponse
	err = json.NewDecoder(resp.Body).Decode(&slackResp)
	if err != nil {
		return err
	}

	if !slackResp.Ok {
		return fmt.Errorf(slackResp.Error)
	}

	return nil
}

func NewApp(queueSize int, httpClient *http.Client, metrics *Metrics, channelOverride string) *App {
	return &App{
		slackQueue:      make(chan SlackPostMessageRequest, queueSize),
		messenger:       &SlackClient{client: httpClient},
		metrics:         metrics,
		channelOverride: channelOverride,
	}
}

func (app *App) Shutdown() {
	close(app.slackQueue)
	// Very important to wait, so that we process all the messages in the queue before exiting!
	app.wg.Wait()
}

//nolint:gocognit // but could probably use a refactor.
func (app *App) processQueue(ctx context.Context, maxRetries int, initialBackoffMs int, slackPostMessageURL string, tokenFlag string, burst int) {
	// This is the rate limiter, which will block until it is allowed to continue on r.Wait(ctx).
	// I kept the rate at 1 per second, as doing more than that will cause Slack to reject the messages anyways. We can burst however.
	// Do note that this is best effort, in case of failures, we will exponentially backoff and retry, which will cause the rate to be lower than 1 per second due to obvious reasons.
	r := rate.NewLimiter(rate.Every(1*time.Second), burst)

	for {
		select {
		case msg, ok := <-app.slackQueue:
			// We do check if the channel is closed, but its important to note is that the channel will be closed when the queue is empty and the Shutdown() is called.
			// Simply calling close(app.slackQueue) will not close the channel, it will only prevent any more messages from being sent to the channel.
			// Only once the channel is empty, will it be closed.
			if !ok {
				return
			}
			log.S(log.Debug, "Got message from queue", log.Any("message", msg))

			// Rate limiter was initially before fetching a message from the queue, but that caused problems by indefinitely looping even if there was no message in the queue.
			// On shutdown, it would cancel the context, even if the queue was stopped (thus no messages would even come in).
			err := r.Wait(ctx)
			if err != nil {
				log.Fatalf("Error while waiting for rate limiter. This should not happen, provide debug info + error message to an issue if it does: %v", err)
				return
			}

			// Update the queue size metric after any change on the queue size
			app.metrics.QueueSize.With(nil).Set(float64(len(app.slackQueue)))

			retryCount := 0
			for {
				// Check if the channel is in the doNotProcessChannels map, if it is, check if it's been more than 15 minutes since we last tried to send a message to it.
				if (doNotProcessChannels[msg.Channel] != time.Time{}) {
					if time.Since(doNotProcessChannels[msg.Channel]) >= 15*time.Minute {
						// Remove the channel from the map, so that we can process it again. If the channel isn't created in the meantime, we will just add it again.
						delete(doNotProcessChannels, msg.Channel)
					} else {
						log.S(log.Info, "Channel is on the doNotProcess list, not trying to post this message", log.String("channel", msg.Channel))
						app.metrics.RequestsNotProcessed.WithLabelValues(msg.Channel).Inc()
						break
					}
				}

				err := app.messenger.PostMessage(msg, slackPostMessageURL, tokenFlag)
				//nolint:nestif // but simplify by not having else at least.
				if err != nil {
					retryable, pause, description := CheckError(err.Error(), msg.Channel)

					if pause {
						log.S(log.Info, "Channel not found, pausing for 15 minutes", log.String("channel", msg.Channel))
						app.metrics.RequestsNotProcessed.WithLabelValues(msg.Channel).Inc()
						break
					}

					if !retryable {
						app.metrics.RequestsFailedTotal.WithLabelValues(msg.Channel).Inc()
						log.S(log.Error, "Permanent error, message will not be retried", log.Any("err", err), log.String("description", description), log.String("channel", msg.Channel), log.Any("message", msg))
						break
					}

					if description == "Unknown error" {
						log.S(log.Error, "Unknown error, since we can't infer what type of error it is, we will retry it. However, please create a ticket/issue for this project for this error", log.Any("err", err))
					}
					log.S(log.Warning, "Temporary error, message will be retried", log.Any("err", err), log.String("description", description), log.String("channel", msg.Channel), log.Any("message", msg))

					app.metrics.RequestsRetriedTotal.WithLabelValues(msg.Channel).Inc()

					if retryCount < maxRetries {
						retryCount++
						backoffDuration := time.Duration(initialBackoffMs*int(math.Pow(2, float64(retryCount-1)))) * time.Millisecond
						time.Sleep(backoffDuration)
					} else {
						log.S(log.Error, "Message failed after retries", log.Any("err", err), log.Int("retryCount", retryCount))
						app.metrics.RequestsFailedTotal.WithLabelValues(msg.Channel).Inc()
						break
					}
				} else {
					log.Debugf("Message sent successfully")
					app.metrics.RequestsSucceededTotal.WithLabelValues(msg.Channel).Inc()
					break
				}
			}

			// Need to call this to clean up the wg, which is vital for the shutdown to work (so that we process all the messages in the queue before exiting cleanly)
			app.wg.Done()

		case <-ctx.Done():
			return
		}
	}
}
