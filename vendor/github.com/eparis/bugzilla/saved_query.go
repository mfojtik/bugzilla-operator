package bugzilla

import (
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const userAgent string = "bugzilla go client"

// newHTTPRequest creates HTTP request
func newHTTPRequest(method string, urlStr string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, urlStr, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept-Charset", "utf-8")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Cache-Control", "no-cache")
	return req, nil
}

// bzBug summary information from bugzilla ticket
type bzBug struct {
	XMLName     xml.Name `xml:"bug"`
	ID          int      `xml:"id"`
	URL         string   `xml:"about,attr"`
	Product     string   `xml:"product"`
	Component   string   `xml:"component"`
	Assignee    string   `xml:"assigned_to"`
	Status      string   `xml:"bug_status"`
	Resolution  string   `xml:"resolution"`
	Description string   `xml:"short_desc"`
	Changed     string   `xml:"changeddate"`
	Severity    string   `xml:"bug_severity"`
	PMScore     int      `xml:"Ccf_pm_score"`
}

func parseBugzCSV(reader io.Reader) (results []bzBug, err error) {
	bs, _ := ioutil.ReadAll(reader)
	csvreader := csv.NewReader(bytes.NewBuffer(bs))

	// ignore first line header
	cNames, _ := csvreader.Read()

	for {
		line, err := csvreader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}

		colData := make(map[string]string)

		for i, data := range line {
			colData[cNames[i]] = data
		}

		bzId := getInt(colData, "Bug ID", 0)
		results = append(results, bzBug{
			ID:          bzId,
			Product:     get(colData, "Product"),
			Component:   get(colData, "Component"),
			Assignee:    get(colData, "Assignee"),
			Status:      get(colData, "Status"),
			Resolution:  get(colData, "Resolution"),
			Description: get(colData, "Summary"),
			Changed:     get(colData, "Changed"),
			PMScore:     getInt(colData, "PM Score", 0),
			Severity:    get(colData, "Severity"),
		})
	}
	return results, nil
}

func get(colData map[string]string, col string) string {
	data, ok := colData[col]
	if !ok {
		return ""
	} else {
		return data
	}
}

func getInt(colData map[string]string, col string, defaultVal int) int {
	str := get(colData, col)
	num, err := strconv.Atoi(str)
	if err == nil {
		return num
	} else {
		return defaultVal
	}
}

func newBugFromBzBug(protoBug bzBug) (bug *Bug, err error) {
	bug = &Bug{
		ID:         protoBug.ID,
		URL:        protoBug.URL,
		Product:    protoBug.Product,
		Component:  []string{protoBug.Component},
		AssignedTo: protoBug.Assignee,
		Status:     protoBug.Status,
		Resolution: protoBug.Resolution,
		Summary:    protoBug.Description,
		Severity:   protoBug.Severity,
	}

	parser := &combinedParser{}
	t, err := parser.parse(protoBug.Changed)
	if err != nil {
		return nil, err
	}
	bug.LastChangeTime = t.String()
	return bug, nil
}

// BugList takes a
// cmdtype=dorem&remaction=run&namedcmd=openshift-group-b-stale&sharer_id=290313
func (c *client) BugList(queryName, sharerID string) ([]Bug, error) {
	if c.cgiClient == nil {
		return nil, fmt.Errorf("BugList() is only supported with CGI client")
	}
	u, err := url.Parse(c.endpoint)
	if err != nil {
		return nil, err
	}
	u.Path = "buglist.cgi"

	v := u.Query()

	v.Add("cmdtype", "dorem")
	v.Add("remaction", "run")
	v.Add("namedcmd", queryName)
	v.Add("sharer_id", sharerID)
	v.Add("ctype", "csv")
	v.Add("human", "1")
	u.RawQuery = v.Encode()
	queryUrl := u.String()

	v.Del("ctype")
	v.Del("human")
	u.RawQuery = v.Encode()
	referer := u.String()

	res, err := c.cgiClient.authenticated(func() (*http.Response, error) {
		req, err := newHTTPRequest("GET", queryUrl, nil)
		if err == nil {
			req.Header.Set("Upgrade-Insecure-Request", "1")
			req.Header.Set("DNT", "1")
			// For some weird reason it doesn't return the right list if no Referer is set
			if referer != "" {
				req.Header.Set("Referer", referer)
			}
		}
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "text/csv")

		res, err := c.cgiClient.httpClient.Do(req)
		if err != nil {
			if strings.Contains(err.Error(), "use of closed network connection") {
				return nil, fmt.Errorf("timeout occured while accessing %v", req.URL)
			}
			return nil, err
		}
		return res, nil
	})
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	bugList, err := parseBugzCSV(res.Body)

	if err != nil {
		return nil, err
	}
	results := make([]Bug, len(bugList))
	for i := range bugList {
		b, err := newBugFromBzBug(bugList[i])
		if err != nil {
			return nil, err
		}
		results[i] = *b
	}
	return results, err
}
