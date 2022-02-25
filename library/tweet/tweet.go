// Package tweet provides primitives for making requests to Twitter API
// and parsing the tweets.
package tweet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/boltdb/bolt"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/rodionlim/tweet/library/log"
)

// Tweets contains a list of tweets and the metadata required for pagination.
type Tweets struct {
	Data []map[string]string `json:"data"`
	Meta Meta                `json:"meta"`
}

// Meta is the metadata from the response of a twitter get request.
type Meta struct {
	NewestID    string `json:"newest_id"`
	OldestID    string `json:"oldest_id"`
	ResultCount int    `json:"result_count"`
	NextToken   string `json:"next_token"`
}

// Req consists of relevant parameters to encapsulate a Twitter API GET request.
type Req struct {
	schemeHost    string
	endpoint      string
	queryParams   url.Values
	usersFilter   []string
	keywordFilter []string
	tweets        Tweets
}

// Get prepares the request and fires it to Twitter API, storing the results in Req object.
func (r *Req) Get() (*Tweets, error) {
	logger := log.Ctx(context.Background())
	token, exists := getBearerToken()
	if !exists {
		logger.Error("Unable to query twitter. TWITTER_BEARER_TOKEN env variable not set.")
		return nil, errors.New("authentication error")
	}
	r.parseQuery()
	req, err := http.NewRequest(http.MethodGet, r.urlString(), nil)
	if err != nil {
		logger.Error(err)
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	logger.Infof("Send req: %v\n", req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		logger.Error(err)
		return nil, err
	}
	logger.Infof("Recv headers: %v\n", resp.Header)

	body, _ := io.ReadAll(resp.Body)
	json.Unmarshal(body, &r.tweets)
	logger.Infof("%+v", r.tweets)
	return &r.tweets, nil
}

// StoreLatestSearchTerm stores the latest keywords used for querying, persistently to disk,
// which can be useful to inform the user what is currently being searched.
func (r *Req) StoreLatestSearchTerms() error {
	logger := log.Ctx(context.Background())
	dir, err := homedir.Dir()
	if err != nil {
		logger.Error(err)
		return err
	}

	logger.Info("Opening bolt db file [tweet.db]")
	basedir := path.Join(dir, "AppData", "tweet")
	if _, err = os.Stat(basedir); errors.Is(err, os.ErrNotExist) {
		err := os.MkdirAll(basedir, os.ModePerm)
		if err != nil {
			logger.Error(err)
		}
	}
	db, err := bolt.Open(path.Join(basedir, "tweet.db"), 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		logger.Error(err)
		return err
	}
	defer db.Close()

	kw := r.keywordFilter
	return db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("meta"))
		if err != nil {
			logger.Error(err)
			return err
		}
		k := []byte("keywords")
		logger.Infof("Storing latest search terms in bolt db %s\n", kw)
		return b.Put(k, []byte(strings.Join(kw, ",")))
	})
}

// GetLatestSearchTerms fetches the latest keywords stored in disk,
// which can be useful to inform the user what is the current search term.
func GetLatestSearchTerms() ([]string, error) {
	logger := log.Ctx(context.Background())
	dir, err := homedir.Dir()
	if err != nil {
		logger.Error(err)
		return nil, err
	}

	logger.Info("Opening bolt db file [tweet.db]")
	basedir := path.Join(dir, "AppData", "tweet")
	if _, err = os.Stat(basedir); errors.Is(err, os.ErrNotExist) {
		err := os.MkdirAll(basedir, os.ModePerm)
		if err != nil {
			logger.Error(err)
		}
	}
	db, err := bolt.Open(path.Join(basedir, "tweet.db"), 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		logger.Error(err)
		return nil, err
	}
	defer db.Close()

	var prevSlice []string
	err = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("meta"))
		if b == nil {
			logger.Error("Bucket meta does not exist")
			return errors.New("invalid bucket")
		}
		prevBytes := b.Get([]byte("keywords"))
		if prevBytes == nil {
			return errors.New("no latest search terms found")
		}
		prevSlice = strings.Split(string(prevBytes), ",")
		logger.Infof("Fetching previous search terms %v\n", prevSlice)
		return nil
	})
	return prevSlice, err
}

// StoreLatestTweetID stores the latest Twitter ID queried, persistently to disk,
// which can be useful when trying to avoid duplicated queries.
func (r *Req) StoreLatestTweetID() error {
	logger := log.Ctx(context.Background())
	dir, err := homedir.Dir()
	if err != nil {
		logger.Error(err)
		return err
	}

	logger.Info("Opening bolt db file [tweet.db]")
	basedir := path.Join(dir, "AppData", "tweet")
	if _, err = os.Stat(basedir); errors.Is(err, os.ErrNotExist) {
		err := os.MkdirAll(basedir, os.ModePerm)
		if err != nil {
			logger.Error(err)
		}
	}
	db, err := bolt.Open(path.Join(basedir, "tweet.db"), 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		logger.Error(err)
		return err
	}
	defer db.Close()

	id := r.tweets.Meta.NewestID
	if id == "" {
		logger.Warn("current req object has no newest id, skip storing newest id")
		return errors.New("invalid id")
	}
	return db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("meta"))
		if err != nil {
			logger.Error(err)
			return err
		}
		k := []byte("newest")
		prevNewestID := b.Get(k)
		logger.Infof("Fetching previous newest tweet id[%s]\n", string(prevNewestID))
		if prevNewestID != nil && string(prevNewestID) >= id {
			// Nothing to cache if previous cached is greater than current latest tweet
			return nil
		}
		logger.Infof("Storing latest tweet id in bolt db [%s]\n", id)
		return b.Put(k, []byte(id))
	})
}

// GetLatestTweetID fetches the latest Twitter ID stored in disk,
// which can be useful when trying to avoid duplicated queries.
func (r *Req) GetLatestTweetID() (string, error) {
	logger := log.Ctx(context.Background())
	dir, err := homedir.Dir()
	if err != nil {
		logger.Error(err)
		return "", err
	}

	logger.Info("Opening bolt db file [tweet.db]")
	basedir := path.Join(dir, "AppData", "tweet")
	if _, err = os.Stat(basedir); errors.Is(err, os.ErrNotExist) {
		err := os.MkdirAll(basedir, os.ModePerm)
		if err != nil {
			logger.Error(err)
		}
	}
	db, err := bolt.Open(path.Join(basedir, "tweet.db"), 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		logger.Error(err)
		return "", err
	}
	defer db.Close()

	var prev string
	err = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("meta"))
		if b == nil {
			logger.Error("Bucket meta does not exist")
			return errors.New("invalid bucket")
		}
		prevBytes := b.Get([]byte("newest"))
		if prevBytes == nil {
			return errors.New("no latest tweet id found")
		}
		prev = string(prevBytes)
		logger.Infof("Fetching previous newest tweet id[%s]\n", prev)
		return nil
	})
	return prev, err
}

func (r *Req) urlString() string {
	return fmt.Sprintf("%s%s?%s", r.schemeHost, r.endpoint, r.queryParams.Encode())
}

// query param has some additional logic when parsing.
func (r *Req) parseQuery() {
	uf := ""
	if len(r.usersFilter) > 0 {
		uf = "(from:" + strings.Join(r.usersFilter, " OR ") + ")"
	}

	kf := "(" + strings.Join(r.keywordFilter, " OR ") + ")"

	if len(uf) > 0 || len(kf) > 0 {
		r.setQueryParam("query", kf+uf)
	}
}

func getBearerToken() (token string, exists bool) {
	token, exists = os.LookupEnv("TWITTER_BEARER_TOKEN")
	return
}

// NewReq is a struct that contains the necessary API parameters before firing the call to Twitter endpoints
func NewReq(opts ...func(r *Req) *Req) *Req {
	req := &Req{
		schemeHost:  "https://api.twitter.com",
		endpoint:    "/2/tweets/search/recent",
		queryParams: make(url.Values),
	}
	req.setQueryParam("tweet.fields", "created_at,text")
	req.setQueryParam("max_results", "10")
	for _, opt := range opts {
		req = opt(req)
	}
	return req
}

// WithEndpoint changes the twitter api endpoint.
func WithEndpoint(endpoint string) func(r *Req) *Req {
	return func(r *Req) *Req {
		r.endpoint = endpoint
		return r
	}
}

// WithUser adds a filter for twitter users.
func WithUsers(users []string) func(r *Req) *Req {
	return func(r *Req) *Req {
		r.usersFilter = users
		return r
	}
}

// WithKeywords adds a filter for keywords in tweets.
func WithKeywords(keywords []string) func(r *Req) *Req {
	var escapedKeywords []string
	for _, w := range keywords {
		// Match exact phrase, i.e. "central bank"
		if len(strings.Split(w, " ")) > 1 {
			w = fmt.Sprintf("%q", w)
		}
		escapedKeywords = append(escapedKeywords, w)
	}

	return func(r *Req) *Req {
		r.keywordFilter = escapedKeywords
		return r
	}
}

// WithSinceTweetID adds a filter for only returning tweets greater than tweet id.
func WithSinceTweetID(id int64) func(r *Req) *Req {
	return func(r *Req) *Req {
		r.setQueryParam("since_id", fmt.Sprint(id))
		return r
	}
}

// WithMaxResults adds a filter for maximum results returned.
func WithMaxResults(cnt int) func(r *Req) *Req {
	return func(r *Req) *Req {
		r.setQueryParam("max_results", fmt.Sprint(cnt))
		return r
	}
}

// Allow setting of since tweet ID at runtime to support polling for latest results without duplicates.
func (r *Req) SetSinceTweetID(v string) {
	r.queryParams.Set("since_id", v)
}

func (r *Req) setQueryParam(k string, v string) {
	r.queryParams.Set(k, v)
}
