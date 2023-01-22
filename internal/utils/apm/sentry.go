package apm

import (
	"log"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
)

func SetupSenery() {
	err := sentry.Init(sentry.ClientOptions{
		Dsn: "https://7328b699ac334448ae3d7411e36b23a4@o4504093435756544.ingest.sentry.io/4504093440802816",
		// Enable printing of SDK debug messages.
		// Useful when getting started or trying to figure something out.
		Debug:            true,
		AttachStacktrace: true,
		// Set TracesSampleRate to 1.0 to capture 100%
		// of transactions for performance monitoring.
		// We recommend adjusting this value in production,
		EnableTracing:    true,
		TracesSampleRate: 1.0,
		TracesSampler: sentry.TracesSampler(func(ctx sentry.SamplingContext) float64 {
			// // As an example, this custom sampler does not send some
			// // transactions to Sentry based on their name.
			// hub := sentry.GetHubFromContext(ctx.Span.Context())
			// name := hub.Scope().Transaction()
			// if name == "GET /favicon.ico" {
			// 	return 0.0
			// }
			// if strings.HasPrefix(name, "HEAD") {
			// 	return 0.0
			// }
			// // As an example, sample some transactions with a uniform rate.
			// if strings.HasPrefix(name, "POST") {
			// 	return 0.2
			// }
			// // Sample all other transactions for testing. On
			// // production, use TracesSampleRate with a rate adequate
			// // for your traffic, or use the SamplingContext to
			// // customize sampling per-transaction.
			return 1.0
		}),
		BeforeSend: func(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
			// Here you can inspect/modify non-transaction events (for example, errors) before they are sent.
			// Returning nil drops the event.
			log.Printf("BeforeSend event [%s]", event.EventID)
			return event
		},
		BeforeSendTransaction: func(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
			// Here you can inspect/modify transaction events before they are sent.
			// Returning nil drops the event.
			if strings.Contains(event.Message, "test-transaction") {
				// Drop the transaction
				return nil
			}
			event.Message += " [example]"
			log.Printf("BeforeSendTransaction event [%s]", event.EventID)
			return event
		},
	})

	if err != nil {
		log.Fatalf("sentry.Init: %s", err)
	}
	// Flush buffered events before the program terminates.
	defer sentry.Flush(2 * time.Second)
}
