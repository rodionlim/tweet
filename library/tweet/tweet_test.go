package tweet_test

import "github.com/rodionlim/tweet/library/tweet"

func Example() {
	// Quick start example - This will query twitter for keywords oil, copper ... and user @Bloomberg Markets
	keywords := []string{"oil", "copper", "rates", "inflation", "gold", "nickel", "powell", "fed", "bonds", "metals", "equities", "energy", "central bank", "commodities", "fx"}
	users := []string{"markets"}

	req := tweet.NewReq(tweet.WithUsers(users), tweet.WithKeywords(keywords))
	req.Get()
}
