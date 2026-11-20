// Package fakegcp provides in-process fakes of the Cloud SQL Admin (REST) and
// Cloud Monitoring (gRPC) APIs. Tests point the real Google client libraries at
// these fakes, so request construction and response decoding are exercised
// end to end without touching real cloud resources.
package fakegcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"

	"google.golang.org/api/option"
	sqladmin "google.golang.org/api/sqladmin/v1"
)

// SQLAdmin is a fake Cloud SQL Admin API backed by an httptest server.
type SQLAdmin struct {
	srv *httptest.Server

	mu         sync.Mutex
	instances  map[string]*sqladmin.DatabaseInstance
	order      []string              // list order of instances
	operations []*sqladmin.Operation // newest first, as the real API returns
	opStatus   map[string]*sqladmin.Operation
	updates    []Update // every PUT received
	requests   []Request
	fail       map[string]int // "METHOD path-suffix" -> HTTP status to return

	// ListPageSize, when > 0, splits instances.list into pages of this size.
	ListPageSize int
}

// Update records a single instances.update (PUT) call.
type Update struct {
	Instance string
	Body     *sqladmin.DatabaseInstance
}

// Request records an inbound request for assertions on query parameters.
type Request struct {
	Method string
	Path   string
	Query  map[string][]string
}

// NewSQLAdmin starts the fake. Call Close when done.
func NewSQLAdmin() *SQLAdmin {
	f := &SQLAdmin{
		instances: map[string]*sqladmin.DatabaseInstance{},
		opStatus:  map[string]*sqladmin.Operation{},
		fail:      map[string]int{},
	}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	return f
}

// Close stops the server.
func (f *SQLAdmin) Close() { f.srv.Close() }

// ClientOptions returns options that point sqladmin.NewService at the fake.
func (f *SQLAdmin) ClientOptions() []option.ClientOption {
	return []option.ClientOption{
		option.WithEndpoint(f.srv.URL + "/"),
		option.WithoutAuthentication(),
		option.WithHTTPClient(f.srv.Client()),
	}
}

// AddInstance registers an instance returned by get/list.
func (f *SQLAdmin) AddInstance(inst *sqladmin.DatabaseInstance) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.instances[inst.Name]; !ok {
		f.order = append(f.order, inst.Name)
	}
	f.instances[inst.Name] = inst
}

// AddOperation appends an operation to operations.list (append in newest-first order).
func (f *SQLAdmin) AddOperation(op *sqladmin.Operation) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.operations = append(f.operations, op)
}

// SetOperationResult controls what operations.get returns for the operation
// created by the next update. Status "" means DONE with no error.
func (f *SQLAdmin) SetOperationResult(op *sqladmin.Operation) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.opStatus["next"] = op
}

// FailWith makes requests matching method and a path suffix return status.
// Example: FailWith("PUT", "/instances/db1", 409).
func (f *SQLAdmin) FailWith(method, pathSuffix string, status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fail[method+" "+pathSuffix] = status
}

// Updates returns all PUTs received so far.
func (f *SQLAdmin) Updates() []Update {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Update(nil), f.updates...)
}

// Requests returns all requests received so far.
func (f *SQLAdmin) Requests() []Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Request(nil), f.requests...)
}

func (f *SQLAdmin) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, Request{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query()})
	for key, status := range f.fail {
		parts := strings.SplitN(key, " ", 2)
		if parts[0] == r.Method && strings.HasSuffix(r.URL.Path, parts[1]) {
			f.mu.Unlock()
			writeError(w, status)
			return
		}
	}
	f.mu.Unlock()

	// Paths: /v1/projects/{p}/instances[/{i}], /v1/projects/{p}/operations[/{op}]
	seg := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(seg) < 4 || seg[0] != "v1" || seg[1] != "projects" {
		http.NotFound(w, r)
		return
	}
	switch {
	case seg[3] == "instances" && len(seg) == 4 && r.Method == http.MethodGet:
		f.listInstances(w, r)
	case seg[3] == "instances" && len(seg) == 5 && r.Method == http.MethodGet:
		f.getInstance(w, seg[4])
	case seg[3] == "instances" && len(seg) == 5 && r.Method == http.MethodPut:
		f.updateInstance(w, r, seg[4])
	case seg[3] == "operations" && len(seg) == 4 && r.Method == http.MethodGet:
		f.listOperations(w, r)
	case seg[3] == "operations" && len(seg) == 5 && r.Method == http.MethodGet:
		f.getOperation(w, seg[4])
	default:
		http.NotFound(w, r)
	}
}

func (f *SQLAdmin) listInstances(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	start := 0
	if tok := r.URL.Query().Get("pageToken"); tok != "" {
		start, _ = strconv.Atoi(tok)
	}
	end := len(f.order)
	resp := &sqladmin.InstancesListResponse{}
	if f.ListPageSize > 0 && start+f.ListPageSize < end {
		end = start + f.ListPageSize
		resp.NextPageToken = strconv.Itoa(end)
	}
	for _, name := range f.order[start:end] {
		resp.Items = append(resp.Items, f.instances[name])
	}
	writeJSON(w, resp)
}

func (f *SQLAdmin) getInstance(w http.ResponseWriter, name string) {
	f.mu.Lock()
	inst, ok := f.instances[name]
	f.mu.Unlock()
	if !ok {
		writeError(w, http.StatusNotFound)
		return
	}
	writeJSON(w, inst)
}

func (f *SQLAdmin) updateInstance(w http.ResponseWriter, r *http.Request, name string) {
	var body sqladmin.DatabaseInstance
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updates = append(f.updates, Update{Instance: name, Body: &body})
	opName := fmt.Sprintf("op-update-%d", len(f.updates))
	result := &sqladmin.Operation{Name: opName, Status: "DONE", OperationType: "UPDATE", TargetId: name}
	if next, ok := f.opStatus["next"]; ok {
		cp := *next
		cp.Name = opName
		result = &cp
	}
	f.opStatus[opName] = result
	writeJSON(w, &sqladmin.Operation{Name: opName, Status: "PENDING", OperationType: "UPDATE", TargetId: name})
}

func (f *SQLAdmin) listOperations(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ops := f.operations
	if inst := r.URL.Query().Get("instance"); inst != "" {
		var filtered []*sqladmin.Operation
		for _, op := range ops {
			if op.TargetId == inst {
				filtered = append(filtered, op)
			}
		}
		ops = filtered
	}
	if mr, err := strconv.Atoi(r.URL.Query().Get("maxResults")); err == nil && mr > 0 && mr < len(ops) {
		ops = ops[:mr]
	}
	writeJSON(w, &sqladmin.OperationsListResponse{Items: ops})
}

func (f *SQLAdmin) getOperation(w http.ResponseWriter, name string) {
	f.mu.Lock()
	op, ok := f.opStatus[name]
	f.mu.Unlock()
	if !ok {
		writeError(w, http.StatusNotFound)
		return
	}
	writeJSON(w, op)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"error":{"code":%d,"message":"fake error %d"}}`, status, status)
}
