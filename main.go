package main

/*

I want a summary of my activity on github in 2021.

- PRs pushed
- PRs reviewed
- Issues opened and closed

Looks like this may be difficult to do across all repos.
Perhaps we can just target one repo at a time?
I mostly know the repos I worked on during the year.

*/

import (
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "log"
    "os"
    "strings"
)

// Having to define these up front is a pain...
type User struct {
    Login string
}

type Event struct {
    Actor User
    Event string
}

type Commit struct {
    Author User
}

type Pull struct {
    Number int
    HtmlUrl string `json:"html_url"`
    MergedAt string `json:"merged_at"`
    State string
    Title string
    User User

    myContribution string
}

//
// The cache functions work around Github API rate limiting
//
func readCache(path string) (data []byte, err error) {
    file, err := os.Open(path)
    if err != nil {
        return nil, err
    }

    data, err = io.ReadAll(file)
    if err != nil {
        return nil, err
    }

    file.Close()
    return data, nil
}

func writeCache(path string, data []byte) {
    // Why can't I just iterate over the chars and compare them to "/"?
    subdir := path[:strings.LastIndex(path, "/")]
    err := os.MkdirAll(subdir, 0700)
    if err != nil {
        log.Fatalf("could not mkdir %s", subdir)
    }
    err = os.WriteFile(path, data, 0700)
    if err != nil {
        log.Fatalf("unable to write to %s", path)
    }
}

func httpGet(url string) []byte {
    resp, err := http.Get(url)
    if err != nil {
        log.Fatal(err)
    }
    body, err := io.ReadAll(resp.Body)
    resp.Body.Close()
    if (resp.StatusCode > 299) {
        log.Fatalf("HTTP %d", resp.StatusCode)
    }
    return body
}

func loadCommit(repo string, sha string) Commit {
    var commit Commit

    jsonFilename := fmt.Sprintf("cache/%s/commits/%s.json", repo, sha)
    data, err := readCache(jsonFilename)
    if err == nil {
        json.Unmarshal(data, &commit)
        return commit
    }

    log.Printf("cache miss, reading from the wire")

    body := httpGet(fmt.Sprintf("https://api.github.com/repos/%s/commits/%s", repo, sha))
    writeCache(jsonFilename, body)

    if err := json.Unmarshal(body, &commit); err != nil {
        log.Fatalf("JSON unmarshalling failed: %s", err)
    }

    return commit
}

func loadEvents(repo string, issueNumber int) []Event {
    var events []Event

    jsonFilename := fmt.Sprintf("cache/%s/events/%d.json", repo, issueNumber)
    data, err := readCache(jsonFilename)
    if err == nil {
        json.Unmarshal(data, &events)
        return events
    }

    log.Printf("cache miss, reading from the wire")

    data = httpGet(fmt.Sprintf("https://api.github.com/repos/%s/issues/%d/events", repo, issueNumber))
    writeCache(jsonFilename, data)

    if err := json.Unmarshal(data, &events); err != nil {
        log.Fatalf("JSON unmarshalling failed: %s", err)
    }

    return events
}

func loadPulls(repo string, page int) []Pull {
    var pulls []Pull

    jsonFilename := fmt.Sprintf("cache/%s/pulls/%d.json", repo, page)
    data, err := readCache(jsonFilename)
    if err == nil {
        json.Unmarshal(data, &pulls)
        return pulls
    }

    log.Printf("cache miss, reading from the wire")

    body := httpGet(fmt.Sprintf("https://api.github.com/repos/%s/pulls?state=all", repo))
    writeCache(jsonFilename, body)

    if err := json.Unmarshal(body, &pulls); err != nil {
        log.Fatalf("JSON unmarshalling failed: %s", err)
    }

    return pulls
}

func whoMerged(repo string, issueNumber int) User {
    for _, event := range(loadEvents(repo, issueNumber)) {
        if event.Event == "merged" {
            return event.Actor
        }
    }
    return User{"nobody"}
}

func main() {
    var repos = os.Args[1:]
    for _, repo := range(repos) {
        // TODO: pagination
        pulls := loadPulls(repo, 1)
        for _, p := range(pulls) {
            //
            // For each pull request, we need to work out what our contribution,
            // if any, actually was.  Did we actually author the PR?  Or did we
            // simply merge it?
            //
            if p.User.Login == "mpenkov" {
                p.myContribution = "authored"
            } else if p.State == "closed" && whoMerged(repo, p.Number).Login == "mpenkov" {
                p.myContribution = "merged"
            }

            if p.myContribution != "" {
                fmt.Printf("%d. [%s] %s %s %s\n", p.Number, p.myContribution, p.MergedAt, p.Title, p.HtmlUrl)
            }
        }
    }
}
