package kvservice

import (
	"context"
	"log"
	"net"
	"net/http"

	"github.com/eliben/raft/part3/raft"
)

type KVService struct {
	id         int
	rs         *raft.Server
	commitChan chan raft.CommitEntry

	ds *DataStore

	srv *http.Server
}

// New creates a new KVService
//
//   - id: this service's ID within its Raft cluster
//   - peerIds: the IDs of the other Raft peers in the cluster
//   - readyChan: notification channel that has to be closed when the Raft
//     cluster is ready (all peers are up and connected to each other).
func New(id int, peerIds []int, readyChan <-chan any) *KVService {
	commitChan := make(chan raft.CommitEntry)

	// raft.Server handles the Raft RPCs in the cluster; after Serve is called,
	// it's ready to accept RPC connections from peers.
	rs := raft.NewServer(id, peerIds, raft.NewMapStorage(), readyChan, commitChan)
	rs.Serve()
	return &KVService{
		id:         id,
		rs:         rs,
		commitChan: commitChan,
		ds:         NewDataStore(),
	}
}

func (kvs *KVService) ConnectToRaftPeer(peerId int, addr net.Addr) error {
	return kvs.rs.ConnectToPeer(peerId, addr)
}

func (kvs *KVService) GetRaftListenAddr() net.Addr {
	return kvs.rs.GetListenAddr()
}

// ServeHTTP starts serving the KV REST API on the given TCP port. This
// function does not block; it fires up the HTTP server and returns. To properly
// shut down the server, call the Shutdown method.
func (kvs *KVService) ServeHTTP(port string) {
	if kvs.srv != nil {
		panic("ServeHTTP called with existing server")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /get/", kvs.handleGet)
	mux.HandleFunc("POST /put/", kvs.handlePut)

	kvs.srv = &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	go func() {
		if err := kvs.srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
}

// Shutdown performs a proper shutdown of the service; it disconnects from
// all Raft peers, shuts down the Raft RPC server, and shuts down the main
// HTTP service. It only returns once shutdown is complete.
func (kvs *KVService) Shutdown() error {
	kvs.rs.DisconnectAll()
	kvs.rs.Shutdown()
	return kvs.srv.Shutdown(context.Background())
}

func (kvs *KVService) handleGet(w http.ResponseWriter, req *http.Request) {

}

func (kvs *KVService) handlePut(w http.ResponseWriter, req *http.Request) {

}
