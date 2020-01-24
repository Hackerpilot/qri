// Package api implements a JSON-API for interacting with a qri node
package api

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	stdlog "log"
	"net"
	"net/http"
	"net/rpc"
	"time"

	golog "github.com/ipfs/go-log"
	"github.com/qri-io/apiutil"
	"github.com/qri-io/qri/lib"
	"github.com/qri-io/qri/version"
)

var log = golog.Logger("qriapi")

// APIVersion is the version string that is written in API responses
var APIVersion = version.String

// LocalHostIP is the IP address for localhost
const LocalHostIP = "127.0.0.1"

func init() {
	// We don't use the log package, and the net/rpc package spits out some complaints b/c
	// a few methods don't conform to the proper signature (comment this out & run 'qri connect' to see errors)
	// so we're disabling the log package for now. This is potentially very stupid.
	// TODO (b5): remove dep on net/rpc package entirely
	stdlog.SetOutput(ioutil.Discard)

	golog.SetLogLevel("qriapi", "info")
}

// Server wraps a qri p2p node, providing traditional access via http
// Create one with New, start it up with Serve
type Server struct {
	*lib.Instance
}

// New creates a new qri server from a p2p node & configuration
func New(inst *lib.Instance) (s Server) {
	return Server{Instance: inst}
}

// Serve starts the server. It will block while the server is running
func (s Server) Serve(ctx context.Context) (err error) {
	node := s.Node()
	cfg := s.Config()

	server := &http.Server{}
	mux := NewServerRoutes(s)
	server.Handler = mux

	go s.MaybeServeRPC(ctx)
	go s.MaybeServeWebsocket(ctx)

	info := fmt.Sprintf("\n📡 Qri v%s\n connect is running. Connection details:\n", APIVersion)
	info += cfg.SummaryString()

	if cfg.P2P != nil && cfg.P2P.Enabled {
		if err := s.Instance.Connect(ctx); err != nil {
			return err
		}

		info += "IPFS Addresses:"
		for _, a := range node.EncapsulatedAddresses() {
			info = fmt.Sprintf("%s\n  %s", info, a.String())
		}
	}
	info += "\n\n"

	s.Node().LocalStreams.Print(info)

	if cfg.API.DisconnectAfter != 0 {
		log.Infof("disconnecting after %d seconds", cfg.API.DisconnectAfter)
		go func(s *http.Server, t int) {
			<-time.After(time.Second * time.Duration(t))
			log.Infof("disconnecting")
			s.Close()
		}(server, cfg.API.DisconnectAfter)
	}

	go func() {
		<-ctx.Done()
		log.Info("shutting down")
		server.Close()
	}()

	// http.ListenAndServe will not return unless there's an error
	return StartServer(cfg.API, server)
}

// MaybeServeRPC checks for a configured RPC port, and registers a listner if so
func (s Server) MaybeServeRPC(ctx context.Context) {
	cfg := s.Config()
	if !cfg.RPC.Enabled {
		return
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", LocalHostIP, cfg.RPC.Port))
	if err != nil {
		log.Infof("RPC listen on port %d error: %s", cfg.RPC.Port, err)
		return
	}

	for _, rcvr := range lib.Receivers(s.Instance) {
		if err := rpc.Register(rcvr); err != nil {
			log.Errorf("cannot start RPC: error registering RPC receiver %s: %s", rcvr.CoreRequestsName(), err.Error())
			return
		}
	}

	go func() {
		<-ctx.Done()
		log.Info("closing RPC")
		listener.Close()
	}()

	rpc.Accept(listener)
	return
}

// HandleIPFSPath responds to IPFS Hash requests with raw data
func (s *Server) HandleIPFSPath(w http.ResponseWriter, r *http.Request) {
	if s.Config().API.ReadOnly {
		readOnlyResponse(w, "/ipfs/")
		return
	}

	s.fetchCAFSPath(r.URL.Path, w, r)
}

func (s Server) fetchCAFSPath(path string, w http.ResponseWriter, r *http.Request) {
	file, err := s.Node().Repo.Store().Get(r.Context(), path)
	if err != nil {
		apiutil.WriteErrResponse(w, http.StatusInternalServerError, err)
		return
	}

	io.Copy(w, file)
}

// HandleIPNSPath resolves an IPNS entry
func (s Server) HandleIPNSPath(w http.ResponseWriter, r *http.Request) {
	node := s.Node()
	if s.Config().API.ReadOnly {
		readOnlyResponse(w, "/ipns/")
		return
	}

	namesys, err := node.GetIPFSNamesys()
	if err != nil {
		apiutil.WriteErrResponse(w, http.StatusBadRequest, fmt.Errorf("no IPFS node present: %s", err.Error()))
		return
	}

	p, err := namesys.Resolve(r.Context(), r.URL.Path[len("/ipns/"):])
	if err != nil {
		apiutil.WriteErrResponse(w, http.StatusBadRequest, fmt.Errorf("error resolving IPNS Name: %s", err.Error()))
		return
	}

	file, err := node.Repo.Store().Get(r.Context(), p.String())
	if err != nil {
		apiutil.WriteErrResponse(w, http.StatusInternalServerError, err)
		return
	}

	io.Copy(w, file)
}

// helper function
func readOnlyResponse(w http.ResponseWriter, endpoint string) {
	apiutil.WriteErrResponse(w, http.StatusForbidden, fmt.Errorf("qri server is in read-only mode, access to '%s' endpoint is forbidden", endpoint))
}

// HealthCheckHandler is a basic ok response for load balancers & co
// returns the version of qri this node is running, pulled from the lib package
func HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{ "meta": { "code": 200, "status": "ok", "versionzz":"` + APIVersion + `" }, "data": [] }`))
}

// NewServerRoutes returns a Muxer that has all API routes
func NewServerRoutes(s Server) *http.ServeMux {
	node := s.Node()
	cfg := s.Config()

	m := http.NewServeMux()

	m.Handle("/health", s.middleware(HealthCheckHandler))
	m.Handle("/ipfs/", s.middleware(s.HandleIPFSPath))
	m.Handle("/ipns/", s.middleware(s.HandleIPNSPath))

	proh := NewProfileHandlers(s.Instance, cfg.API.ReadOnly)
	m.Handle("/me", s.middleware(proh.ProfileHandler))
	m.Handle("/profile", s.middleware(proh.ProfileHandler))
	m.Handle("/profile/photo", s.middleware(proh.ProfilePhotoHandler))
	m.Handle("/profile/poster", s.middleware(proh.PosterHandler))

	ph := NewPeerHandlers(node, cfg.API.ReadOnly)
	m.Handle("/peers", s.middleware(ph.PeersHandler))
	m.Handle("/peers/", s.middleware(ph.PeerHandler))
	m.Handle("/connect/", s.middleware(ph.ConnectToPeerHandler))
	m.Handle("/connections", s.middleware(ph.ConnectionsHandler))

	if cfg.Remote != nil && cfg.Remote.Enabled {
		log.Info("running in `remote` mode")

		remh := NewRemoteHandlers(s.Instance)
		m.Handle("/remote/dsync", s.middleware(remh.DsyncHandler))
		m.Handle("/remote/logsync", s.middleware(remh.LogsyncHandler))
		m.Handle("/remote/refs", s.middleware(remh.RefsHandler))
	}

	dsh := NewDatasetHandlers(s.Instance, cfg.API.ReadOnly)
	m.Handle("/list", s.middleware(dsh.ListHandler))
	m.Handle("/list/", s.middleware(dsh.PeerListHandler))
	m.Handle("/save", s.middleware(dsh.SaveHandler))
	m.Handle("/save/", s.middleware(dsh.SaveHandler))
	m.Handle("/remove/", s.middleware(dsh.RemoveHandler))
	m.Handle("/me/", s.middleware(dsh.GetHandler))
	m.Handle("/add/", s.middleware(dsh.AddHandler))
	m.Handle("/rename", s.middleware(dsh.RenameHandler))
	m.Handle("/export/", s.middleware(dsh.ZipDatasetHandler))
	m.Handle("/diff", s.middleware(dsh.DiffHandler))
	m.Handle("/body/", s.middleware(dsh.BodyHandler))
	m.Handle("/stats/", s.middleware(dsh.StatsHandler))
	m.Handle("/unpack/", s.middleware(dsh.UnpackHandler))

	remClientH := NewRemoteClientHandlers(s.Instance, cfg.API.ReadOnly)
	m.Handle("/publish/", s.middleware(remClientH.PublishHandler))
	m.Handle("/fetch/", s.middleware(remClientH.NewFetchHandler("/fetch")))

	uh := UpdateHandlers{
		UpdateMethods: lib.NewUpdateMethods(s.Instance),
		ReadOnly:      cfg.API.ReadOnly,
	}
	m.Handle("/update", s.middleware(uh.UpdatesHandler))
	m.Handle("/update/run", s.middleware(uh.RunHandler))
	m.Handle("/update/logs", s.middleware(uh.LogsHandler))
	m.Handle("/update/logs/file", s.middleware(uh.LogFileHandler))
	m.Handle("/update/service", s.middleware(uh.ServiceHandler))

	fsih := NewFSIHandlers(s.Instance, cfg.API.ReadOnly)
	m.Handle("/status/", s.middleware(fsih.StatusHandler("/status")))
	m.Handle("/init/", s.middleware(fsih.InitHandler("/init")))
	m.Handle("/checkout/", s.middleware(fsih.CheckoutHandler("/checkout")))
	m.Handle("/restore/", s.middleware(fsih.RestoreHandler("/restore")))
	m.Handle("/fsi/write/", s.middleware(fsih.WriteHandler("/fsi/write")))

	renderh := NewRenderHandlers(node.Repo)
	m.Handle("/render/", s.middleware(renderh.RenderHandler))

	lh := NewLogHandlers(node)
	m.Handle("/history/", s.middleware(lh.LogHandler))

	rch := NewRegistryClientHandlers(s.Instance, cfg.API.ReadOnly)
	m.Handle("/registry/profile/new", s.middleware(rch.CreateProfileHandler))
	m.Handle("/registry/profile/prove", s.middleware(rch.ProveProfileKeyHandler))

	sh := NewSearchHandlers(s.Instance)
	m.Handle("/search", s.middleware(sh.SearchHandler))

	rh := NewRootHandler(dsh, ph)
	m.Handle("/", s.datasetRefMiddleware(s.middleware(rh.Handler)))

	return m
}
