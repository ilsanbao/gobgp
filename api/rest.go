// Copyright (C) 2014 Nippon Telegraph and Telephone Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package api

import (
	"encoding/json"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
	"net/http"
	"os"
	"strconv"
)

var logger *log.Logger = &log.Logger{
	Out:       os.Stderr,
	Formatter: new(log.JSONFormatter),
	Hooks:     make(map[log.Level][]log.Hook),
	Level:     log.InfoLevel,
}

const (
	_ = iota
	REQ_NEIGHBOR
	REQ_NEIGHBORS
	REQ_ADJ_RIB_IN
	REQ_ADJ_RIB_OUT
	REQ_ADJ_RIB_LOCAL
	REQ_ADJ_RIB_LOCAL_BEST
)
const (
	BASE_VERSION       = "/v1"
	NEIGHBOR           = "/bgp/neighbor"
	NEIGHBORS          = "/bgp/neighbors"
	ADJ_RIB_IN         = "/bgp/adj-rib-in"
	ADJ_RIB_OUT        = "/bgp/adj-rib-out"
	ADJ_RIB_LOCAL      = "/bgp/adj-rib-local"
	ADJ_RIB_LOCAL_BEST = "/bgp/adj-rib-local/best"

	PARAM_REMOTE_PEER_ADDR = "remotePeerAddr"
)

const REST_PORT = 8080

const (
	_ = iota
	JSON_FORMATTED
	JSON_UN_FORMATTED
)

var JsonFormat int = JSON_FORMATTED

// trigger struct for exchanging information in the rest and peer.
// rest and peer operated at different thread.

type RestRequest struct {
	RequestType int
	RemoteAddr  string
	ResponseCh  chan RestResponse
	Err         error
}

func NewRestRequest(reqType int, remoteAddr string) *RestRequest {
	r := &RestRequest{
		RequestType: reqType,
		RemoteAddr:  remoteAddr,
		ResponseCh:  make(chan RestResponse),
	}
	return r
}

type RestResponse interface {
	Err() error
}

type RestResponseDefault struct {
	ResponseErr error
}

func (r *RestResponseDefault) Err() error {
	return r.ResponseErr
}

// Response struct for Neighbor
type RestResponseNeighbor struct {
	RestResponseDefault
	RemoteAddr    string
	RemoteAs      uint32
	NeighborState uint32
	UpdateCount   int
}

// Response struct for Rib
type RestResponseRib struct {
	RestResponseDefault
	RemoteAddr string
	RemoteAs   uint32
	RibInfo    []string
}

type RestServer struct {
	port        int
	bgpServerCh chan *RestRequest
}

func NewRestServer(port int, bgpServerCh chan *RestRequest) *RestServer {
	rs := &RestServer{
		port:        port,
		bgpServerCh: bgpServerCh}
	return rs
}

// Main thread of rest service.
// URL than can receive.
//   get state of neighbor.
//     -- curt -i -X GET http://<ownIP>:3000/v1/bgp/neighbor/<remote address of target neighbor>
//   get state of neighbors.
//     -- curt -i -X GET http://<ownIP>:3000/v1/bgp/neighbors
//   get adj-rib-in of each neighbor.
//     -- curt -i -X GET http://<ownIP>:3000/v1/bgp/adj-rib-in/<remote address of target neighbor>
//   get adj-rib-out of each neighbor.
//     -- curt -i -X GET http://<ownIP>:3000/v1/bgp/adj-rib-out/<remote address of target neighbor>
//   get adj-rib-local of each neighbor.
//     -- curt -i -X GET http://<ownIP>:3000/v1/bgp/adj-rib-local/<remote address of target neighbor>
//   get only best path of adj-rib-local  of each neighbor.
//     -- curt -i -X GET http://<ownIP>:3000/v1/bgp/adj-rib-local/best/<remote address of target neighbor>
func (rs *RestServer) Serve() {
	neighbor := BASE_VERSION + NEIGHBOR
	// neighbors := BASE_VERSION + NEIGHBORS
	// adjRibIn := BASE_VERSION + ADJ_RIB_IN
	// adjRibOut := BASE_VERSION + ADJ_RIB_OUT
	// adjRibLocal := BASE_VERSION + ADJ_RIB_LOCAL
	// adjRibLocalBest := BASE_VERSION + ADJ_RIB_LOCAL_BEST

	r := mux.NewRouter()
	// set URLs
	r.HandleFunc(neighbor+"/{"+PARAM_REMOTE_PEER_ADDR+"}", rs.Neighbor).Methods("GET")
	// r.HandleFunc(neighbors, rs.Neighbors).Methods("GET")
	// r.HandleFunc(adjRibIn+"/{"+PARAM_REMOTE_PEER_ADDR+"}", rs.AdjRibIn).Methods("GET")
	// r.HandleFunc(adjRibOut+"/{"+PARAM_REMOTE_PEER_ADDR+"}", rs.AdjRibOut).Methods("GET")
	// r.HandleFunc(adjRibLocal+"/{"+PARAM_REMOTE_PEER_ADDR+"}", rs.AdjRibLocal).Methods("GET")
	// r.HandleFunc(adjRibLocalBest+"/{"+PARAM_REMOTE_PEER_ADDR+"}", rs.AdjRibLocalBest).Methods("GET")

	// Handler when not found url
	r.NotFoundHandler = http.HandlerFunc(NotFoundHandler)
	http.Handle("/", r)

	http.ListenAndServe(":"+strconv.Itoa(rs.port), nil)

}

// Http request of curl, return the json format infomation of neibor state.
func (rs *RestServer) Neighbor(w http.ResponseWriter, r *http.Request) {
	//	remotePeerAddr := mux.Vars(r)[PARAM_REMOTE_PEER_ADDR]

	params := mux.Vars(r)
	remoteAddr, found := params[PARAM_REMOTE_PEER_ADDR]
	if !found {
		errStr := "neighbor address is not specified"
		logger.Debug(errStr)
		http.Error(w, errStr, http.StatusInternalServerError)
	}

	logger.Debugf("Look up neighbor with the remote address : %v", remoteAddr)

	//Send channel of request parameter.
	req := NewRestRequest(REQ_NEIGHBOR, remoteAddr)
	rs.bgpServerCh <- req

	//Wait response
	resInf := <-req.ResponseCh
	if e := resInf.Err(); e != nil {
		logger.Debug(e.Error())
		http.Error(w, e.Error(), http.StatusInternalServerError)
		return
	}

	res := resInf.(*RestResponseNeighbor)

	var jns []byte
	var err error
	switch JsonFormat {
	case JSON_FORMATTED:
		jns, err = json.MarshalIndent(res, "", "  ") // formatted json
	case JSON_UN_FORMATTED:
		jns, err = json.Marshal(res) // Unformatted json
	}
	if err != nil {
		errStr := fmt.Sprintf("Failed to perth json of neighbor state", remoteAddr)
		logger.Error(errStr)
		http.Error(w, errStr, http.StatusInternalServerError)
		return
	}
	w.Write(jns)
}

func NotFoundHandler(w http.ResponseWriter, r *http.Request) {
	http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
}