package rest

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/evergreen-ci/sink"
	"github.com/evergreen-ci/sink/model"
	"github.com/evergreen-ci/sink/units"
	"github.com/gorilla/mux"
	"github.com/mongodb/amboy"
	"github.com/mongodb/curator/sthree"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/tychoish/gimlet"
)

////////////////////////////////////////////////////////////////////////
//
// GET /status

type StatusResponse struct {
	Revision     string           `json:"revision"`
	QueueStats   amboy.QueueStats `json:"queue,omitempty"`
	QueueRunning bool             `json:"running"`
}

// statusHandler processes the GET request for
func (s *Service) statusHandler(w http.ResponseWriter, r *http.Request) {
	resp := &StatusResponse{Revision: sink.BuildRevision}

	if s.queue != nil {
		resp.QueueRunning = s.queue.Started()
		resp.QueueStats = s.queue.Stats()
	}

	gimlet.WriteJSON(w, resp)
}

////////////////////////////////////////////////////////////////////////
//
// GET /status/events/{level}?limit=<int>

type SystemEventsResponse struct {
	Level  string         `json:"level,omitempty"`
	Total  int            `json:"total,omitempty"`
	Count  int            `json:"count,omitempty"`
	Events []*model.Event `json:"events"`
	Err    string         `json:"error"`
}

func (s *Service) getSystemEvents(w http.ResponseWriter, r *http.Request) {
	l := gimlet.GetVars(r)["level"]
	resp := &SystemEventsResponse{}

	if l == "" {
		resp.Err = "no level specified"
		gimlet.WriteErrorJSON(w, resp)
		return
	}

	if !level.IsValidPriority(level.FromString(l)) {
		resp.Err = fmt.Sprintf("%s is not a valid level", l)
	}
	resp.Level = l

	limitArg := r.URL.Query()["limit"][0]
	limit, err := strconv.Atoi(limitArg)
	if err != nil {
		resp.Err = fmt.Sprintf("%s is not a valid limit [%s]", limitArg, err.Error())
		gimlet.WriteErrorJSON(w, resp)
		return
	}

	e := &model.Events{}
	err = e.FindLevel(l, limit)
	if err != nil {
		resp.Err = "problem running query for events"
		gimlet.WriteErrorJSON(w, resp)
		return
	}

	resp.Events = e.Slice()
	resp.Total, err = e.CountLevel(l)
	if err != nil {
		resp.Err = fmt.Sprintf("problem fetching errors: %+v", err)
		gimlet.WriteErrorJSON(w, resp)
		return
	}
	resp.Count = len(resp.Events)
	gimlet.WriteJSON(w, resp)
}

////////////////////////////////////////////////////////////////////////
//
// GET /status/events/{id}

type SystemEventResponse struct {
	ID    string       `json:"id"`
	Error string       `json:"error"`
	Event *model.Event `json:"event"`
}

func (s *Service) getSystembEvent(w http.ResponseWriter, r *http.Request) {
	id := gimlet.GetVars(r)["id"]
	resp := &SystemEventResponse{}
	if id == "" {
		resp.Error = "id not specified"
		gimlet.WriteErrorJSON(w, resp)
		return
	}
	resp.ID = id

	event := &model.Event{}
	if err := event.FindID(id); err != nil {
		resp.Error = err.Error()
		gimlet.WriteErrorJSON(w, resp)
		return
	}

	resp.Event = event
	gimlet.WriteJSON(w, resp)
}

////////////////////////////////////////////////////////////////////////
//
// POST /status/events/{id}/acknowledge
//
// (nothing is read from the body)

func (s *Service) acknowledgeSystemEvent(w http.ResponseWriter, r *http.Request) {
	id := gimlet.GetVars(r)["id"]
	resp := &SystemEventResponse{}
	if id == "" {
		resp.Error = "id not specified"
		gimlet.WriteErrorJSON(w, resp)
		return
	}
	resp.ID = id

	event := &model.Event{}
	if err := event.FindID(id); err != nil {
		resp.Error = err.Error()
		gimlet.WriteErrorJSON(w, resp)
		return
	}
	resp.Event = event

	if err := event.Acknowledge(); err != nil {
		resp.Error = err.Error()
		gimlet.WriteErrorJSON(w, resp)
		return
	}

	gimlet.WriteJSON(w, resp)
}

////////////////////////////////////////////////////////////////////////
//
// POST /simple_log/{id}
//
// body: { "inc": <int>, "ts": <date>, "content": <str> }

type simpleLogRequest struct {
	Time      time.Time `json:"ts"`
	Increment int       `json:"inc"`
	Content   string    `json:"content"`
}

type SimpleLogInjestionResponse struct {
	Errors []string `json:"errors,omitempty"`
	JobID  string   `json:"jobId,omitempty"`
	LogID  string   `json:"logId"`
}

func (s *Service) simpleLogInjestion(w http.ResponseWriter, r *http.Request) {
	req := &simpleLogRequest{}
	resp := &SimpleLogInjestionResponse{}
	resp.LogID = gimlet.GetVars(r)["id"]
	defer r.Body.Close()

	if resp.LogID == "" {
		resp.Errors = []string{"no log id specified"}
		gimlet.WriteErrorJSON(w, resp)
		return
	}

	if err := gimlet.GetJSON(r.Body, req); err != nil {
		grip.Error(err)
		resp.Errors = append(resp.Errors, err.Error())
		gimlet.WriteErrorJSON(w, resp)
		return
	}

	j := units.MakeSaveSimpleLogJob(resp.LogID, req.Content, req.Time, req.Increment)
	resp.JobID = j.ID()

	if err := s.queue.Put(j); err != nil {
		grip.Error(err)
		resp.Errors = append(resp.Errors, err.Error())
		gimlet.WriteInternalErrorJSON(w, resp)
		return
	}

	gimlet.WriteJSON(w, resp)
}

////////////////////////////////////////////////////////////////////////
//
// GET /simple_log/{id}

type SimpleLogContentResponse struct {
	LogID string   `json:"logId"`
	Error string   `json:"err,omitempty"`
	URLS  []string `json:"urls"`
}

// simpleLogRetrieval takes in a log id and returns the log documents associated with that log id.
func (s *Service) simpleLogRetrieval(w http.ResponseWriter, r *http.Request) {
	resp := &SimpleLogContentResponse{}

	resp.LogID = gimlet.GetVars(r)["id"]
	if resp.LogID == "" {
		resp.Error = "no log specified"
		gimlet.WriteErrorJSON(w, resp)
		return
	}
	allLogs := &model.LogSegments{}

	if err := allLogs.Find(resp.LogID, false); err != nil {
		resp.Error = err.Error()
		gimlet.WriteErrorJSON(w, resp)
		return
	}

	for _, l := range allLogs.Slice() {
		resp.URLS = append(resp.URLS, l.URL)
	}

	gimlet.WriteJSON(w, resp)
}

////////////////////////////////////////////////////////////////////////
//
// GET /simple_log/{id}/text

func (s *Service) simpleLogGetText(w http.ResponseWriter, r *http.Request) {
	id := gimlet.GetVars(r)["id"]
	allLogs := &model.LogSegments{}

	if err := allLogs.Find(id, true); err != nil {
		gimlet.WriteErrorText(w, err.Error())
		return
	}

	var bucket *sthree.Bucket
	for _, l := range allLogs.Slice() {
		if bucket.String() != l.Bucket {
			bucket = sthree.GetBucket(l.Bucket)
		}

		data, err := bucket.Read(l.KeyName)
		if err != nil {
			grip.Warning(err)
			gimlet.WriteInternalErrorText(w, err.Error())
			return
		}

		gimlet.WriteText(w, data)
	}
}

////////////////////////////////////////////////////////////////////////
//
// POST /system_info/
//
// body: json produced by grip/message.SystemInfo documents

type SystemInfoReceivedResponse struct {
	ID        string    `json:"id,omitempty"`
	Hostname  string    `json:"host,omitempty"`
	Timestamp time.Time `json:"time,omitempty"`
	Error     string    `json:"err,omitempty"`
}

func (s *Service) recieveSystemInfo(w http.ResponseWriter, r *http.Request) {
	resp := &SystemInfoReceivedResponse{}
	req := message.SystemInfo{}
	defer r.Body.Close()

	if err := gimlet.GetJSON(r.Body, &req); err != nil {
		grip.Error(err)
		resp.Error = err.Error()
		gimlet.WriteErrorJSON(w, resp)
		return
	}

	data := &model.SystemInformationRecord{
		Data:      req,
		Hostname:  req.Hostname,
		Timestamp: req.Time,
	}

	if data.Timestamp.IsZero() {
		data.Timestamp = time.Now()
	}

	if err := data.Insert(); err != nil {
		grip.Error(err)
		resp.Error = err.Error()
		gimlet.WriteErrorJSON(w, resp)
		return
	}

	resp.ID = string(data.ID)
	gimlet.WriteJSON(w, resp)
}

////////////////////////////////////////////////////////////////////////
//
// GET /system_info/host/{hostname}?start=[timestamp]<,end=[timestamp],limit=[num]>

type SystemInformationResponse struct {
	Error string                `json:"error,omitempty"`
	Data  []*message.SystemInfo `json:"data"`
	Total int                   `json:"total,omitempty"`
	Limit int                   `json:"limit,omitempty"`
}

func (s *Service) fetchSystemInfo(w http.ResponseWriter, r *http.Request) {
	resp := &SystemInformationResponse{}
	host := gimlet.GetVars(r)["host"]
	if host == "" {
		resp.Error = "no host specified"
		gimlet.WriteErrorJSON(w, resp)
		return
	}

	startArg := r.FormValue("start")
	if startArg == "" {
		resp.Error = "no start time argument"
		gimlet.WriteErrorJSON(w, resp)
		return
	}

	start, err := time.Parse(time.RFC3339, startArg)
	if err != nil {
		resp.Error = fmt.Sprintf("could not parse time string '%s' in to RFC3339: %+v",
			startArg, err.Error())
		gimlet.WriteErrorJSON(w, resp)
		return
	}

	limitArg := r.FormValue("limit")
	if limitArg != "" {
		resp.Limit, err = strconv.Atoi(limitArg)
		if err != nil {
			resp.Error = err.Error()
			gimlet.WriteErrorJSON(w, resp)
			return
		}
	} else {
		resp.Limit = 100
	}

	end := time.Now()
	endArg := r.FormValue("end")
	if endArg != "" {
		end, err = time.Parse(time.RFC3339, endArg)
		if err != nil {
			resp.Error = err.Error()
			gimlet.WriteErrorJSON(w, resp)
			return
		}
	}

	out := &model.SystemInformationRecords{}
	count, err := out.CountHostname(host)
	if err != nil {
		resp.Error = fmt.Sprintf("could not count '%s' host: %s", host, err.Error())
		gimlet.WriteErrorJSON(w, resp)
		return
	}
	resp.Total = count

	err = out.FindHostnameBetween(host, start, end, resp.Limit)
	if err != nil {
		resp.Error = fmt.Sprintf("could not retrieve results, %s", err.Error())
		gimlet.WriteErrorJSON(w, resp)
		return
	}

	for _, d := range out.Slice() {
		resp.Data = append(resp.Data, &d.Data)
	}

	gimlet.WriteJSON(w, resp)
}

////////////////////////////////////////////////////////////////////////
//
// POST /depgraph/{id}

type createDepGraphResponse struct {
	Error   string `json:"error,omitempty"`
	ID      string `json:"id,omitempty"`
	Created bool   `json:"created"`
}

func (s *Service) createDepGraph(w http.ResponseWriter, r *http.Request) {
	resp := createDepGraphResponse{}
	id := mux.Vars(r)["id"]
	g := &model.GraphMetadata{}
	g.Find(id)
	if g.IsNil() {
		g.BuildID = id
		if err := g.Insert(); err != nil {
			resp.Error = err.Error()
			gimlet.WriteErrorJSON(w, resp)
			return
		}
		resp.Created = true
	}

	resp.ID = g.BuildID
	gimlet.WriteJSON(w, resp)
}

////////////////////////////////////////////////////////////////////////
//
// GET /depgraph/{id}

type depGraphResolvedRespose struct {
	Nodes []*model.GraphNode `json:"nodes"`
	Edges []*model.GraphEdge `json:"edges"`
	Error string             `json:"error,omitempty"`
	ID    string             `json:"id"`
}

func (s *Service) resolveDepGraph(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	resp := depGraphResolvedRespose{ID: id}
	g := &model.GraphMetadata{}

	if err := g.Find(id); err != nil {
		resp.Error = err.Error()
		gimlet.WriteErrorJSON(w, resp)
		return
	}

	catcher := grip.NewCatcher()

	nodes, err := g.AllNodes()
	catcher.Add(err)

	edges, err := g.AllEdges()
	catcher.Add(err)

	if catcher.HasErrors() {
		resp.Error = catcher.Resolve().Error()
		gimlet.WriteErrorJSON(w, resp)
		return
	}

	resp.Edges = edges
	resp.Nodes = nodes

	gimlet.WriteJSON(w, resp)
}

////////////////////////////////////////////////////////////////////////
//
// POST /depgraph/{id}/nodes

func (s *Service) addDepGraphNodes(w http.ResponseWriter, r *http.Request) {
}

////////////////////////////////////////////////////////////////////////
//
// GET /depgraph/{id}/nodes

type depGraphNodesRespose struct {
	Nodes []*model.GraphNode `json:"nodes"`
	Error string             `json:"error,omitempty"`
	ID    string             `json:"id"`
}

func (s *Service) getDepGraphNodes(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	resp := depGraphNodesRespose{ID: id}
	g := &model.GraphMetadata{}

	if err := g.Find(id); err != nil {
		resp.Error = err.Error()
		gimlet.WriteErrorJSON(w, resp)
		return
	}

	nodes, err := g.AllNodes()
	if err != nil {
		resp.Error = err.Error()
		gimlet.WriteErrorJSON(w, resp)
		return
	}

	resp.Nodes = nodes
	gimlet.WriteJSON(w, resp)
}

////////////////////////////////////////////////////////////////////////
//
// POST /depgraph/{id}/edges

func (s *Service) addDepGraphEdges(w http.ResponseWriter, r *http.Request) {

}

////////////////////////////////////////////////////////////////////////
//
// GET /depgraph/{id}/edges

type depGraphEdgesRespose struct {
	Edges []*model.GraphEdge `json:"edges"`
	Error string             `json:"error,omitempty"`
	ID    string             `json:"id"`
}

func (s *Service) getDepGraphEdges(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	resp := depGraphEdgesRespose{ID: id}
	g := &model.GraphMetadata{}

	if err := g.Find(id); err != nil {
		resp.Error = err.Error()
		gimlet.WriteErrorJSON(w, resp)
		return
	}

	edges, err := g.AllEdges()
	if err != nil {
		resp.Error = err.Error()
		gimlet.WriteErrorJSON(w, resp)
		return
	}

	resp.Edges = edges
	gimlet.WriteJSON(w, resp)
}
