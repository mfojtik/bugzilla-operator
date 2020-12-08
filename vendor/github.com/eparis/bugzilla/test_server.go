/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package bugzilla

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
)

func (tc *testClient) readData() error {
	if len(tc.bugs) > 0 {
		return nil
	}
	bugJson, err := ioutil.ReadFile(tc.path)
	if err != nil {
		return err
	}
	bugs := []Bug{}
	if err := json.Unmarshal(bugJson, &bugs); err != nil {
		return err
	}
	for i := range bugs {
		bug := bugs[i]
		tc.bugs[bug.ID] = bug
	}
	tc.bugList = BugList{
		Bugs: bugs,
	}
	return nil
}

func (tc *testClient) getBugList() BugList {
	tc.readData()
	return tc.bugList
}

func (tc *testClient) getBugMap() map[int]Bug {
	tc.readData()
	return tc.bugs
}

func (tc *testClient) handleGet(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/rest/bug/"))
	if err != nil {
		fmt.Printf("malformed bug id: %s\n", r.URL.Path)
		http.Error(w, "400 Bad Request", http.StatusBadRequest)
		return
	}
	bugs := tc.getBugMap()
	bug, ok := bugs[id]
	if !ok {
		fmt.Printf("Unable to find test bug: %d\n", id)
		http.Error(w, "404 Not Found", http.StatusNotFound)
	}
	buglist := BugList{
		Bugs: []Bug{bug},
	}
	b, err := json.Marshal(buglist)
	if err != nil {
		fmt.Printf("Unable to marshal bug data: %d: %v\n", id, err)
		http.Error(w, "500 Invalid Bug data", http.StatusInternalServerError)
	}
	w.Write(b)
}

func (tc *testClient) handleQuery(w http.ResponseWriter, r *http.Request) {
	bugs := tc.getBugList()
	b, err := json.Marshal(bugs)
	if err != nil {
		fmt.Printf("Unable to marshal bug data: %v\n", err)
		http.Error(w, "500 Invalid Bug data", http.StatusInternalServerError)
		return
	}
	w.Write(b)
}

func (tc *testClient) getTestServer(path string) *httptest.Server {
	testServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-BUGZILLA-API-KEY") != "api-key" {
			fmt.Printf("did not get api-key passed in X-BUGZILLA-API-KEY header\n")
			http.Error(w, "403 Forbidden", http.StatusForbidden)
			return
		}
		if r.URL.Query().Get("api_key") != "api-key" {
			fmt.Printf("did not get api-key passed in api_key query parameter\n")
			http.Error(w, "403 Forbidden", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet {
			fmt.Printf("incorrect method to get a bug: %s\n", r.Method)
			http.Error(w, "400 Bad Request", http.StatusBadRequest)
			return
		}
		switch {
		case strings.HasPrefix(r.URL.Path, "/rest/bug/"):
			tc.handleGet(w, r)
		case r.URL.Path == "/rest/bug":
			tc.handleQuery(w, r)
		default:
			fmt.Printf("incorrect path to get a bug: %s\n", r.URL.Path)
			http.Error(w, "400 Bad Request", http.StatusBadRequest)
			return
		}
	}))
	tc.endpoint = testServer.URL
	return testServer
}

type testClient struct {
	client
	path    string
	bugs    map[int]Bug
	bugList BugList
}

func (tc testClient) UpdateBug(_ int, _ BugUpdate) error {
	return nil
}

func (tc testClient) Search(query Query) ([]*Bug, error) {
	srv := tc.getTestServer(tc.path)
	defer srv.Close()

	return tc.client.Search(query)
}

func (tc testClient) GetExternalBugPRsOnBug(_ int) ([]ExternalBug, error) {
	return []ExternalBug{}, nil
}

func (tc testClient) GetExternalBugs(_ int) ([]ExternalBug, error) {
	return []ExternalBug{}, nil
}

func (tc testClient) GetBug(id int) (*Bug, error) {
	srv := tc.getTestServer(tc.path)
	defer srv.Close()

	return tc.client.GetBug(id)
}

func (tc testClient) Endpoint() string {
	return tc.path
}

func (testClient) AddPullRequestAsExternalBug(_ int, _ string, _ string, _ int) (bool, error) {
	return false, nil
}

// GetTestClient returns a client which acts on the data in a json file specified in path
func GetTestClient(path string) Client {
	tc := &testClient{
		client: client{
			logger: logrus.WithField("testing", "true"),
			client: &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
				},
			},
			getAPIKey: func() []byte {
				return []byte("api-key")
			},
		},
		path: path,
		bugs: map[int]Bug{},
	}
	return tc
}
