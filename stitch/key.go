package stitch

import (
	"github.com/google/go-github/github"
	"github.com/robertkrimen/otto"
)

var githubCache = make(map[string][]string)

func githubKeys(username string) ([]string, error) {
	if keys, ok := githubCache[username]; ok {
		return keys, nil
	}
	keys, err := GetGithubKeys(username)
	if err != nil {
		return nil, err
	}
	githubCache[username] = keys
	return keys, nil
}

// GetGithubKeys retrieves the GitHub public keys associated with the
// given username.
// Stored in a variable so we can mock it out for the unit tests.
var GetGithubKeys = func(username string) ([]string, error) {
	usersService := github.NewClient(nil).Users
	opt := &github.ListOptions{}
	keys, _, err := usersService.ListKeys(username, opt)

	if err != nil {
		return nil, err
	}

	var keyStrings []string
	for _, key := range keys {
		keyStrings = append(keyStrings, *key.Key)
	}

	return keyStrings, nil
}

func githubKeysImpl(call otto.FunctionCall) otto.Value {
	if len(call.ArgumentList) < 1 {
		panic(call.Otto.MakeRangeError(
			"githubKeys requires the username as an argument"))
	}

	username, err := call.Argument(0).ToString()
	if err != nil {
		panic(err)
	}

	keys, err := githubKeys(username)
	if err != nil {
		stitchError(call.Otto, err)
	}

	keysVal, err := call.Otto.ToValue(keys)
	if err != nil {
		panic(err)
	}

	return keysVal
}
