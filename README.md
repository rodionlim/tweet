# Overview

Tweet is a twitterbot listener and distributor of relevant tweets written in Golang. It comprises of a http server, a UI for custom parameters and a wrapper library that interacts with Twitter API v2.

# Installation

Using Tweet is easy. To use the library, `go get` will install the libraries and dependencies for your project.

```
go get github.com/rodionlim/tweet
```

Later, to receive updates, run

```
go get -u github.com/rodionlim/tweet
```

# Usage

To spin up the webserver and start the twitter notification service

```
# Run in the root of the repository
go run .

# Go to http://localhost:3000 for the UI
```

# Quick Start

The following Go code searches for twitter user `Bloomberg Markets` and any tweets that contains the keywords.

```
keywords := []string{"oil", "copper", "rates", "inflation", "gold", "nickel", "powell", "fed", "bonds", "metals", "equities", "energy", "central bank", "commodities", "fx"}
users := []string{"markets"}

req := tweet.NewReq(tweet.WithUsers(users), tweet.WithKeywords(keywords))
req.Get()
```

# License

Tweet is released under the Apache 2.0 license. See [LICENSE](https://github.com/rodionlim/tweet/blob/master/LICENSE)
