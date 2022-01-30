package main

/*

I want a summary of my activity on github in 2021.

- [x] PRs pushed
- [x] PRs reviewed
- [ ] Issues opened and closed

Looks like this may be difficult to do across all repos.
Perhaps we can just target one repo at a time?
I mostly know the repos I worked on during the year.

*/

import (
    "encoding/json"
    "fmt"
    "html/template"
    "io"
    "net/http"
    "log"
    "os"
    "sort"
    "strings"
    "time"
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
    CreatedAt string `json:"created_at"`
    MergedAt string `json:"merged_at"`
    State string
    Title string
    User User

    MyContribution string
    Timestamp string
}

//
// For sorting
//
type PullList []Pull

func (pl PullList) Len() int {
    return len(pl)
}

func (pl PullList) Less(i, j int) bool {
    return pl[i].Number > pl[j].Number
}

func (pl PullList) Swap(i, j int) {
    pl[i], pl[j] = pl[j], pl[i]
}

type RepoResult struct {
    Name string
    Pulls []Pull
    Authored int
    Merged int
}

const header string = `<html>
<head>
<style>
body {
    font-family: sans-serif;
}

table thead th {
    text-align: left;
}

table {
    border-collapse: collapse;
    text-align: left;
    vertical-align: middle;
}

tbody tr:nth-child(odd) {
    background-color: hsl(0, 100%, 100%);
}

tbody tr:nth-child(even) {
    background-color: hsl(0, 0%, 90%);
}

th, td {
    border: 1px solid black;
    padding: 5px;
}

td.state-open {
    color: hsl(0, 90%, 50%);
}
td.state-closed {
    color: hsl(240, 100%, 50%);
}

td.contribution-authored {
    color: hsl(120, 80%, 40%);
}
td.contribution-merged {
    color: hsl(240, 100%, 50%);
}

td {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 400px;
}
</style>
</head>
<body>`

const templ string = `
<h1>{{ .Name }}</h1>
<p>Authored {{ .Authored }} and merged {{ .Merged }} contributions.</p>
<table>
    <thead>
        <tr>
            <th>#</th>
            <th>Timestamp</th>
            <th>State</th>
            <th>Contribution</th>
            <th>Title</th>
        </tr>
    </thead>
    <tbody>
    {{ range .Pulls }}
        <tr>
            <td><a href="{{ .HtmlUrl }}">{{ .Number }}</a></td>
            <td>{{ .Timestamp }}</td>
            <td class="state-{{ .State }}">{{ .State }}</td>
            <td class="contribution-{{ .MyContribution }}">{{ .MyContribution }}</td>
            <td><a href="{{ .HtmlUrl }}">{{ .Title }}</a></td>
        </tr>
    {{ end }}
    </tbody>
</table>
`

var report = template.Must(template.New("issuelist").Parse(templ))

//
// The cache functions are yet another work-around for Github API rate limiting
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
    //
    // Prevent us from getting rate-limited
    //
    time.Sleep(5000 * time.Millisecond)
    return body
}

// This loadX stuff is rather repetitive, is there a way to avoid duplicating
// it for each of Commit, Pull and Event?
func loadCommit(repo string, sha string) Commit {
    var commit Commit

    jsonFilename := fmt.Sprintf("cache/%s/commits/%s.json", repo, sha)
    data, err := readCache(jsonFilename)
    if err == nil {
        json.Unmarshal(data, &commit)
        return commit
    }

    url := fmt.Sprintf("https://api.github.com/repos/%s/commits/%s", repo, sha)
    log.Printf("cache miss, reading %s from the wire", url)

    body := httpGet(url)
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

    url := fmt.Sprintf("https://api.github.com/repos/%s/issues/%d/events", repo, issueNumber)
    log.Printf("cache miss, reading %s from the wire", url)
    data = httpGet(url)

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

    url := fmt.Sprintf("https://api.github.com/repos/%s/pulls?state=all&page=%d", repo, page)
    log.Printf("cache miss, reading %s from the wire", url)

    data = httpGet(url)
    writeCache(jsonFilename, data)

    if err := json.Unmarshal(data, &pulls); err != nil {
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

func parseTime(pull Pull) time.Time {
    const format string = "2006-01-02T15:04:05Z"
    parsedTime, err := time.Parse(format, pull.CreatedAt)
    if (err != nil) {
        log.Fatalf("unable to parse time from %s", pull.CreatedAt)
    }
    return parsedTime
}

func main() {
    var repos = os.Args[1:]
    fmt.Println(header)
    for _, repo := range(repos) {
        var pulls []Pull
        var done bool = false
        var authored int = 0
        var merged int = 0
        for page := 1; !done; page++ {
            pagePulls := loadPulls(repo, page)
            if len(pagePulls) == 0 {
                break
            }
            sort.Sort(PullList(pagePulls))

            for _, p := range(pagePulls) {
                ts := parseTime(p)
                if ts.Year() > 2021 {
                    continue
                } else if ts.Year() < 2021 {
                    done = true
                    break
                }
                p.Timestamp = ts.Format("2006-01-02")

                //
                // For each pull request, we need to work out what our contribution,
                // if any, actually was.  Did we actually author the PR?  Or did we
                // simply merge it?
                //
                if p.User.Login == "mpenkov" {
                    p.MyContribution = "authored"
                    authored++
                } else if p.State == "closed" && whoMerged(repo, p.Number).Login == "mpenkov" {
                    p.MyContribution = "merged"
                    merged++
                } else {
                    continue
                }

                pulls = append(pulls, p)
            }
        }

        if err := report.Execute(os.Stdout, RepoResult{repo, pulls, authored, merged}); err != nil {
            log.Fatal(err)
        }
    }
}
