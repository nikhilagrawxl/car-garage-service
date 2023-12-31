package gocql

import (
	"net"
	"sync"
	"time"
)

type eventDebouncer struct {
	name   string
	timer  *time.Timer
	mu     sync.Mutex
	events []frame

	callback func([]frame)
	quit     chan struct{}

	logger StdLogger
}

func newEventDebouncer(name string, eventHandler func([]frame), logger StdLogger) *eventDebouncer {
	e := &eventDebouncer{
		name:     name,
		quit:     make(chan struct{}),
		timer:    time.NewTimer(eventDebounceTime),
		callback: eventHandler,
		logger:   logger,
	}
	e.timer.Stop()
	go e.flusher()

	return e
}

func (e *eventDebouncer) stop() {
	e.quit <- struct{}{} // sync with flusher
	close(e.quit)
}

func (e *eventDebouncer) flusher() {
	for {
		select {
		case <-e.timer.C:
			e.mu.Lock()
			e.flush()
			e.mu.Unlock()
		case <-e.quit:
			return
		}
	}
}

const (
	eventBufferSize   = 1000
	eventDebounceTime = 1 * time.Second
)

// flush must be called with mu locked
func (e *eventDebouncer) flush() {
	if len(e.events) == 0 {
		return
	}

	// if the flush interval is faster than the callback then we will end up calling
	// the callback multiple times, probably a bad idea. In this case we could drop
	// frames?
	go e.callback(e.events)
	e.events = make([]frame, 0, eventBufferSize)
}

func (e *eventDebouncer) debounce(frame frame) {
	e.mu.Lock()
	e.timer.Reset(eventDebounceTime)

	// TODO: probably need a warning to track if this threshold is too low
	if len(e.events) < eventBufferSize {
		e.events = append(e.events, frame)
	} else {
		e.logger.Printf("%s: buffer full, dropping event frame: %s", e.name, frame)
	}

	e.mu.Unlock()
}

func (s *Session) handleEvent(framer *framer) {
	frame, err := framer.parseFrame()
	if err != nil {
		s.logger.Printf("gocql: unable to parse event frame: %v\n", err)
		return
	}

	if gocqlDebug {
		s.logger.Printf("gocql: handling frame: %v\n", frame)
	}

	switch f := frame.(type) {
	case *schemaChangeKeyspace, *schemaChangeFunction,
		*schemaChangeTable, *schemaChangeAggregate, *schemaChangeType:

		s.schemaEvents.debounce(frame)
	case *topologyChangeEventFrame, *statusChangeEventFrame:
		s.nodeEvents.debounce(frame)
	default:
		s.logger.Printf("gocql: invalid event frame (%T): %v\n", f, f)
	}
}

func (s *Session) handleSchemaEvent(frames []frame) {
	// TODO: debounce events
	for _, frame := range frames {
		switch f := frame.(type) {
		case *schemaChangeKeyspace:
			s.schemaDescriber.clearSchema(f.keyspace)
			s.handleKeyspaceChange(f.keyspace, f.change)
			s.hostSource.getClusterPartitionInfo()
		case *schemaChangeTable:
			s.schemaDescriber.clearSchema(f.keyspace)
			s.hostSource.getClusterPartitionInfo()
		case *schemaChangeAggregate:
			s.schemaDescriber.clearSchema(f.keyspace)
		case *schemaChangeFunction:
			s.schemaDescriber.clearSchema(f.keyspace)
		case *schemaChangeType:
			s.schemaDescriber.clearSchema(f.keyspace)
		}
	}
}

func (s *Session) handleKeyspaceChange(keyspace, change string) {
	s.control.awaitSchemaAgreement()
	s.policy.KeyspaceChanged(KeyspaceUpdateEvent{Keyspace: keyspace, Change: change})
}

func (s *Session) handleNodeEvent(frames []frame) {
	type nodeEvent struct {
		change string
		host   net.IP
		port   int
	}

	events := make(map[string]*nodeEvent)
	s.hostSource.getClusterPartitionInfo()

	for _, frame := range frames {
		// TODO: can we be sure the order of events in the buffer is correct?
		switch f := frame.(type) {
		case *topologyChangeEventFrame:
			event, ok := events[f.host.String()]
			if !ok {
				event = &nodeEvent{change: f.change, host: f.host, port: f.port}
				events[f.host.String()] = event
			}
			event.change = f.change

		case *statusChangeEventFrame:
			event, ok := events[f.host.String()]
			if !ok {
				event = &nodeEvent{change: f.change, host: f.host, port: f.port}
				events[f.host.String()] = event
			}
			event.change = f.change
		}
	}

	for _, f := range events {
		if gocqlDebug {
			s.logger.Printf("gocql: dispatching event: %+v\n", f)
		}

		switch f.change {
		case "NEW_NODE":
			s.handleNewNode(f.host, f.port)
		case "REMOVED_NODE":
			s.handleRemovedNode(f.host, f.port)
		case "MOVED_NODE":
		// java-driver handles this, not mentioned in the spec
		// TODO(zariel): refresh token map
		case "UP":
			s.handleNodeUp(f.host, f.port)
		case "DOWN":
			s.handleNodeDown(f.host, f.port)
		}
	}
}

func (s *Session) addNewNode(ip net.IP, port int) {
	// Get host info and apply any filters to the host
	hostInfo, err := s.hostSource.getHostInfo(ip, port)
	if err != nil {
		s.logger.Printf("gocql: events: unable to fetch host info for (%s:%d): %v\n", ip, port, err)
		return
	} else if hostInfo == nil {
		// ignore if it's null because we couldn't find it
		return
	}

	if t := hostInfo.Version().nodeUpDelay(); t > 0 {
		time.Sleep(t)
	}

	// should this handle token moving?
	hostInfo = s.ring.addOrUpdate(hostInfo)

	if !s.cfg.filterHost(hostInfo) {
		s.startPoolFill(hostInfo)
	}

	if s.control != nil && !s.cfg.IgnorePeerAddr {
		// TODO(zariel): debounce ring refresh
		s.hostSource.refreshRing()
	}
}

func (s *Session) handleNewNode(ip net.IP, port int) {
	if gocqlDebug {
		s.logger.Printf("gocql: Session.handleNewNode: %s:%d\n", ip.String(), port)
	}

	ip, port = s.cfg.translateAddressPort(ip, port)

	// if we already have the host and it's already up, then do nothing
	host := s.ring.getHost(ip)
	if host != nil && host.IsUp() {
		return
	}

	s.addNewNode(ip, port)
}

func (s *Session) handleRemovedNode(ip net.IP, port int) {
	if gocqlDebug {
		s.logger.Printf("gocql: Session.handleRemovedNode: %s:%d\n", ip.String(), port)
	}

	ip, port = s.cfg.translateAddressPort(ip, port)

	// we remove all nodes but only add ones which pass the filter
	host := s.ring.getHost(ip)
	if host == nil {
		host = &HostInfo{connectAddress: ip, port: port}
	}
	s.ring.removeHost(ip)

	host.setState(NodeDown)
	if !s.cfg.filterHost(host) {
		s.policy.RemoveHost(host)
		s.pool.removeHost(ip)
	}

	if !s.cfg.IgnorePeerAddr {
		s.hostSource.refreshRing()
	}
}

func (s *Session) handleNodeUp(eventIp net.IP, eventPort int) {
	if gocqlDebug {
		s.logger.Printf("gocql: Session.handleNodeUp: %s:%d\n", eventIp.String(), eventPort)
	}

	ip, port := s.cfg.translateAddressPort(eventIp, eventPort)

	host := s.ring.getHost(ip)
	if host == nil {
		s.addNewNode(ip, port)
		return
	}

	if s.cfg.filterHost(host) {
		return
	}

	if d := host.Version().nodeUpDelay(); d > 0 {
		time.Sleep(d)
	}
	s.startPoolFill(host)
}

func (s *Session) startPoolFill(host *HostInfo) {
	// we let the pool call handleNodeConnected to change the host state
	s.pool.addHost(host)
	s.policy.AddHost(host)
}

func (s *Session) handleNodeConnected(host *HostInfo) {
	if gocqlDebug {
		s.logger.Printf("gocql: Session.handleNodeConnected: %s:%d\n", host.ConnectAddress(), host.Port())
	}

	host.setState(NodeUp)

	if !s.cfg.filterHost(host) {
		s.policy.HostUp(host)
	}
}

func (s *Session) handleNodeDown(ip net.IP, port int) {
	if gocqlDebug {
		s.logger.Printf("gocql: Session.handleNodeDown: %s:%d\n", ip.String(), port)
	}

	host := s.ring.getHost(ip)
	if host == nil {
		host = &HostInfo{connectAddress: ip, port: port}
	}

	host.setState(NodeDown)
	if s.cfg.filterHost(host) {
		return
	}

	s.policy.HostDown(host)
	s.pool.hostDown(ip)
}
