/*
Copyright © 2022 Rodion Lim <rodion.lim@hotmail.com>

*/
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"strings"
	"sync"
	"time"

	"net/http"

	"github.com/rodionlim/tweet/library/log"
	"github.com/rodionlim/tweet/library/notifier"
	"github.com/rodionlim/tweet/library/tweet"
)

var (
	portvar int
	hostvar string
)

type key int

const (
	keyKeywords key = iota
	keyTdata
	keyInterval
	keyNotifierObj
)

type NotifierObj struct {
	notifier   notifier.Notifier
	notifyArgs interface{}
}

func init() {
	flag.IntVar(&portvar, "port", 3000, "Specify a port for the server to listen on")
	flag.StringVar(&hostvar, "host", "localhost", "Specify host of the server, e.g. 10.50.20.118")
}

func main() {
	flag.Parse()

	logger := log.Ctx(context.Background())
	tpl := template.Must(template.ParseFiles("./index.html"))
	tdata := NewTemplateData(hostvar, portvar)

	mux := http.NewServeMux()
	mux.HandleFunc("/favicon.ico", faviconHandler)
	mux.HandleFunc("/start", func(w http.ResponseWriter, req *http.Request) {
		startHandler(w, req, &tdata)
	})
	mux.HandleFunc("/stop", func(w http.ResponseWriter, req *http.Request) {
		stopHandler(w, req, &tdata)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		logger.Infof("Recv req: %v\n", req)
		if req.URL.Path != "/" {
			http.NotFound(w, req)
			return
		}
		tpl.Execute(w, tdata)
	})
	logger.Info(fmt.Sprintf("Listening on %s:%d", hostvar, portvar))
	logger.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", portvar), mux))
}

func startHandler(w http.ResponseWriter, req *http.Request, tdata *templateData) {
	ctx := context.Background()
	logger := log.Ctx(ctx)
	logger.Infof("Recv req: %v\n", req)

	// TODO: shift CORS to a middleware
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")

	if req.Method == "OPTIONS" {
		return
	}

	if req.Method == "POST" {
		// Prevent starting more than once
		tdata.mutex.Lock()
		defer tdata.mutex.Unlock()

		if tdata.Started {
			http.Error(w, "400 Subscription already started", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithCancel(ctx)
		tdata.Cancel = &cancel
		body, _ := io.ReadAll(req.Body)
		keyVal := make(map[string]string)
		json.Unmarshal(body, &keyVal)
		kwStr := keyVal["keywords"]
		kw := strings.Split(kwStr, ",")
		for i := range kw {
			kw[i] = strings.TrimSpace(kw[i])
		}

		// dynamic instantiation of downstream notification service
		slackChannel := keyVal["slackChannelID"]
		var notifierObj *NotifierObj
		if slackChannel != "" {
			notifierObj = &NotifierObj{notifier: notifier.NewSlacker(), notifyArgs: notifier.SlackArgs{ChannelID: keyVal["slackChannelID"]}}
		}

		ctx = context.WithValue(ctx, keyKeywords, kw)
		ctx = context.WithValue(ctx, keyTdata, tdata)
		ctx = context.WithValue(ctx, keyInterval, time.Minute*3)
		ctx = context.WithValue(ctx, keyNotifierObj, notifierObj)

		go start(ctx)

		w.Write([]byte("Success: Started polling tweets"))
	} else {
		http.Error(w, "400 Only POST method is supported", http.StatusBadRequest)
	}
}

func stopHandler(w http.ResponseWriter, req *http.Request, tdata *templateData) {
	ctx := context.Background()
	logger := log.Ctx(ctx)
	logger.Infof("Recv req: %v\n", req)

	// TODO: shift CORS to a middleware
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")

	if req.Method == "OPTIONS" {
		return
	}

	if req.Method == "POST" {
		(*tdata.Cancel)()
		w.Write([]byte("Success: Stopped polling tweets"))
	} else {
		http.Error(w, "400 Only POST method is supported", http.StatusBadRequest)
	}
}

func start(ctx context.Context) {
	logger := log.Ctx(ctx)
	interval := ctx.Value(keyInterval).(time.Duration)
	timer := time.Tick(interval)
	kw := ctx.Value(keyKeywords).([]string)
	tdata := ctx.Value(keyTdata).(*templateData)
	notifierObj := ctx.Value(keyNotifierObj).(*NotifierObj)

	tdata.Started = true
	req := tweet.NewReq(tweet.WithUsers([]string{"markets"}), tweet.WithKeywords(kw))

	run := func(req *tweet.Req) {
		tdata.mutex.Lock()
		id, err := req.GetLatestTweetID()
		if err == nil {
			req.SetSinceTweetID(id)
		}
		tweets, err := req.Get()
		if err != nil {
			logger.Error(err)
		}
		req.StoreLatestTweetID()
		tdata.Tweets = tweets
		tdata.mutex.Unlock()

		for _, tweet := range tweets.Data {
			msg, exists := tweet["text"]
			if !exists {
				continue
			}
			notifierObj.notifier.Notify(msg, notifierObj.notifyArgs)
		}
	}

	logger.Infof("Started polling tweets with params [interval: %s, kw: %v]\n", interval, kw)
	run(req)
	req.StoreLatestSearchTerms()
	st, err := tweet.GetLatestSearchTerms()
	if err != nil {
		logger.Error(err)
	}
	tdata.SearchTerms = st

	for {
		select {
		case <-timer:
			run(req)
		case <-ctx.Done():
			logger.Info("ended polling tweets")
			tdata.mutex.Lock()
			tdata.Started = false
			tdata.mutex.Unlock()
			return
		}
	}
}

func faviconHandler(w http.ResponseWriter, req *http.Request) {
	http.ServeFile(w, req, "favicon.ico")

}

func NewTemplateData(host string, port int) templateData {
	st, _ := tweet.GetLatestSearchTerms()
	return templateData{
		Started:        false,
		SchemeHostPort: fmt.Sprintf("http://%s:%d", host, port),
		SearchTerms:    st,
		mutex:          &sync.Mutex{},
	}
}

type templateData struct {
	Started        bool
	SchemeHostPort string
	SearchTerms    []string
	Tweets         *tweet.Tweets
	Cancel         *context.CancelFunc
	mutex          *sync.Mutex
}
