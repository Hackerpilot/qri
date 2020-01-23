package api

import (
	"context"
	"fmt"

	"net"
	"net/http"
	"time"

	"github.com/qri-io/qri/base/component"
	"github.com/qri-io/qri/p2p"
	"github.com/qri-io/qri/watchfs"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

const qriWebsocketProtocol = "qri-websocket"

// MaybeServeWebsocket creates a websocket that clients can connect to in order
// to get realtime events
func (s Server) MaybeServeWebsocket(ctx context.Context) {
	apiCfg := s.Config().API
	if apiCfg == nil || !apiCfg.Enabled || !apiCfg.WebsocketEnabled {
		return
	}

	// Watch the filesystem. Events will be sent to websocket connections.
	node := s.Node()
	fsmessages, err := s.startFilesysWatcher(node)
	if err != nil {
		log.Infof("Watching filesystem error: %s", err)
		return
	}

	go func() {
		l, err := net.Listen("tcp", fmt.Sprintf("%s:%d", LocalHostIP, apiCfg.WebsocketPort))
		if err != nil {
			log.Infof("Websocket listen on port %d error: %s", apiCfg.WebsocketPort, err)
			return
		}
		defer l.Close()

		// Collect all websocket connections. Should only be one at a time, but that may
		// change in the future.
		connections := []*websocket.Conn{}
		srv := &http.Server{
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
					Subprotocols:       []string{qriWebsocketProtocol},
					InsecureSkipVerify: true,
				})
				if err != nil {
					log.Infof("Websocket accept error: %s", err)
					return
				}
				connections = append(connections, c)
			}),
			ReadTimeout:  time.Second * 15,
			WriteTimeout: time.Second * 15,
		}
		defer srv.Close()

		known := component.GetKnownFilenames()

		// Filesystem events are forwarded to the websocket. In the future, this may be
		// expanded to handle other types of events, such as SaveDatasetProgressEvent,
		// and DiffProgressEvent, but this is fine for now.
		go func() {
			for {
				e := <-fsmessages
				if s.filterEvent(e, known) {
					log.Debugf("filesys event: %s\n", e)
					for k, c := range connections {
						err = wsjson.Write(ctx, c, e)
						if err != nil {
							log.Errorf("connection %d: wsjson write error: %s", k, err)
						}
					}
				}
			}
		}()

		// Start http server for websocket.
		err = srv.Serve(l)
		if err != http.ErrServerClosed {
			log.Infof("failed to listen and serve: %v", err)
		}
	}()
}

func (s Server) startFilesysWatcher(node *p2p.QriNode) (chan watchfs.FilesysEvent, error) {
	refs, err := node.Repo.References(0, 100)
	if err != nil {
		return nil, err
	}
	// Extract fsi paths for all working directories.
	paths := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.FSIPath != "" {
			paths = append(paths, ref.FSIPath)
		}
	}
	// Watch those paths.
	// TODO(dlong): When datasets are init'd, or checked out, or removed, update the watchlist.
	s.Instance.Watcher = watchfs.NewFilesysWatcher()
	fsmessages := s.Instance.Watcher.Begin(paths)
	return fsmessages, nil
}

func (s Server) filterEvent(event watchfs.FilesysEvent, knownFilenames map[string][]string) bool {
	return component.IsKnownFilename(event.Source, knownFilenames)
}
