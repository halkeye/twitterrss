package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/coreos/pkg/flagutil"
	"github.com/dghubble/go-twitter/twitter"
	"github.com/gorilla/feeds"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

type arrayFlags []string

func (i *arrayFlags) String() string {
	return "my string representation"
}

func (i *arrayFlags) Set(value string) error {
	*i = append(*i, value)
	return nil
}

type flagStruct struct {
	consumerKey    string
	consumerSecret string
	port           int
	usernames      arrayFlags
}

func main() {
	flags := flagStruct{}

	flag.Var(&flags.usernames, "usernames", "Allowed Usernames")
	flag.StringVar(&flags.consumerKey, "consumer-key", "", "Twitter Consumer Key")
	flag.StringVar(&flags.consumerSecret, "consumer-secret", "", "Twitter Consumer Secret")
	flag.IntVar(&flags.port, "port", 8000, "port")
	flag.Parse()
	flagutil.SetFlagsFromEnv(flag.CommandLine, "TWITTER")

	if flags.consumerKey == "" || flags.consumerSecret == "" {
		log.Fatal("Application Access Token required")
	}

	if os.Getenv("PORT") != "" {
		port, err := strconv.Atoi(os.Getenv("PORT"))
		if err != nil {
			log.Fatal("Invalid port")
		}
		flags.port = port
	}

	r := mux.NewRouter()
	r.HandleFunc("/healthcheck", HealthCheckHandler)

	for i := 0; i < len(flags.usernames); i++ {
		url := fmt.Sprintf("/feed/%s.xml", flags.usernames[i])
		log.Print(url)
		r.HandleFunc(url, UsernameHandler(flags.usernames[i], flags.consumerKey, flags.consumerSecret))
	}

	loggedRouter := handlers.LoggingHandler(os.Stdout, r)
	log.Printf("Listening on :%d\n", flags.port)
	http.ListenAndServe(fmt.Sprintf(":%d", flags.port), Recovery(handlers.ProxyHeaders(loggedRouter)))
}

func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		defer func() {
			err := recover()
			if err != nil {

				jsonBody, _ := json.Marshal(map[string]string{
					"error": "There was an internal server error",
				})

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				w.Write(jsonBody)

				log.Fatal(err)
			}

		}()

		next.ServeHTTP(w, r)
	})
}

func HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	response := map[string]string{}

	jsonBody, err := json.Marshal(response)
	if err != nil {
		panic(errors.Wrap(err, "Unable to create response"))
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonBody)
}

func UsernameHandler(username string, consumerKey string, consumerSecret string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		// oauth2 configures a client that uses app credentials to keep a fresh token
		config := &clientcredentials.Config{
			ClientID:     consumerKey,
			ClientSecret: consumerSecret,
			TokenURL:     "https://api.twitter.com/oauth2/token",
		}
		// http.Client will automatically authorize Requests
		httpClient := config.Client(oauth2.NoContext)

		// Twitter client
		client := twitter.NewClient(httpClient)

		// Status Show
		tweets, _, err := client.Timelines.UserTimeline(&twitter.UserTimelineParams{
			ScreenName:     username,
			ExcludeReplies: twitter.Bool(true),
		})

		if err != nil {
			panic(errors.Wrap(err, "Unable to get tweets"))
		}

		feed := &feeds.Feed{
			Title:       fmt.Sprintf("%s tweets", username),
			Link:        &feeds.Link{Href: r.URL.Path},
			Description: fmt.Sprintf("%s tweets", username),
			Author:      &feeds.Author{Name: "https://github.com/halkeye/twitterrss"},
			Created:     time.Now(),
		}

		var feedItems []*feeds.Item
		for i := 0; i < len(tweets); i++ {
			tweet := tweets[i]
			createdAt, _ := tweet.CreatedAtTime()
			feedItems = append(feedItems,
				&feeds.Item{
					Id:          tweet.IDStr,
					Title:       tweet.IDStr,
					Link:        &feeds.Link{Href: tweet.Source},
					Description: tweet.Text,
					Created:     createdAt,
				})
		}

		feed.Items = feedItems

		rss, err := feed.ToRss()
		if err != nil {
			panic(errors.Wrap(err, "unable to create rss feed"))
		}

		w.Header().Set("Content-Type", "application/rss+xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(rss))
	}
}
