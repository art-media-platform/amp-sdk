package arc

import (
	"net/url"

	"github.com/arcspace/go-arc-sdk/stdlib/symbol"
	"github.com/arcspace/go-arc-sdk/stdlib/task"
)

// Host allows app and transport services to be attached.
// Child processes attach as it responds to client requests to "pin" cells via URLs.
type Host interface {
	task.Context

	// Offers Go runtime and package level access to this Host's primary symbol and arc.App registry.
	// The arc.Registry interface bakes security and efficiently and tries to serve as effective package manager.
	Registry() Registry

	// StartNewSession creates a new HostSession and binds its TxMsg transport to a stream.
	StartNewSession(parent HostService, via Transport) (HostSession, error)
}

// Transport wraps a Msg transport abstraction, allowing a Host to connect over any data transport layer.
// For example, a tcp-based transport as well as a dll-based transport are both implemented..
type Transport interface {

	// Describes this transport for logging and debugging.
	Label() string

	// Called when this stream should close because the associated parent host session is closing or has closed.
	Close() error

	// SendTx sends a TxMsg to the remote client.
	// ErrStreamClosed is used to denote normal stream close.
	// Like grpc.Transport.SendTx(), on exit, the TxMsg has been copied and so can be reused.
	SendTx(m *TxMsg) error

	// RecvTx blocks until it receives a TxMsg or the stream is done.
	// ErrStreamClosed is used to denote normal stream close.
	RecvTx() (*TxMsg, error)
}

// HostService attaches to a arc.Host as a child, extending host functionality.
// FUTURE: interface HostService goes away, replaced by a host script that starts arc.Apps in a "sys.services" Context.
type HostService interface {
	task.Context

	// StartService attaches a child task to a Host and starts this HostService.
	// This service may retain the arc.Host instance so that it can make calls to StartNewSession().
	StartService(on Host) error

	// GracefulStop initiates a polite stop of this extension and blocks until it's in a "soft" closed state,
	//    meaning that its service has effectively stopped but its Context is still open.
	// Note this could any amount of time (e.g. until all open requests are closed)
	// Typically, GracefulStop() is called (blocking) and then Context.Close().
	// To stop immediately, Context.Close() is always available.
	GracefulStop()
}

// HostSession in an open client session with a Host.
// Closing is initiated via task.Context.Close().
type HostSession interface {
	task.Context    // Underlying task context
	SessionRegistry // How symbols and types registered and resolved

	// Called when this session is newly opened to set up the SessionRegistry
	InitSessionRegistry(symTable symbol.Table)

	// Returns the running AssetPublisher instance for this session.
	AssetPublisher() AssetPublisher

	// Returns info about this user and session
	LoginInfo() Login

	// Sends a readied tx to the client for handling.
	// If msg.ReqID == 0, the attr is sent to the client's session controller (for sending session meta messages).
	// On exit, the given msg should not be referenced further.
	SendTx(tx *TxMsg) error

	// PinCell resolves and pins a requested cell.
	PinCell(req PinReq) (PinContext, error)

	// Gets the currently running AppInstance for an AppID.
	// If the requested app is not running and autoCreate is set, a new instance is created and started.
	GetAppInstance(appID UID, autoCreate bool) (AppInstance, error)
}

// Registry is where apps and types are registered -- concurrency safe.
type Registry interface {

	// Registers an element value type (AttrElemVal) as a prototype under its AttrElemType (also a valid AttrSpec type expression).
	// If an entry already exists (common for a type used by multiple apps), an error is returned and is a no-op.
	// This and ResolveAttrSpec() allow NewElemVal() to work.
	RegisterElemType(prototype AttrElemVal)

	// When a HostSession creates a new SessionRegistry(), this populates it with its registered ElemTypes.
	ExportTo(dst SessionRegistry) error

	// Registers an app by its UUID, URI, and schemas it supports.
	RegisterApp(app *App) error

	// Looks-up an app by UUID -- READ ONLY ACCESS
	GetAppByUID(appUID UID) (*App, error)

	// Selects the app that best matches an invocation string.
	GetAppForInvocation(invocation string) (*App, error)
}

// SessionRegistry manages a HostSession's symbol and type definitions -- concurrency safe.
type SessionRegistry interface {

	// Returns the symbol table for a session a technique sometimes also known as interning.
	ClientSymbols() symbol.Table

	// Translates a native symbol ID to a client symbol ID, returning false if not found.
	NativeToClientID(nativeID uint32) (clientID uint32, found bool)

	// Registers an AttrElemVal as a prototype under its element type name..
	// This and ResolveAttrSpec() allow NewElemVal() to work.
	RegisterElemType(prototype AttrElemVal) error

	// Registers a block of symbol, attr, cell, and selector definitions for a client.
	RegisterDefs(defs *RegisterDefs) error

	// Resolves an AttrSpec into useful symbols, auto-registering the AttrSpec as needed.
	// Typically used during AppInstance.OnNew() to get the AttrIDs that correspond to the AttrSpecs it will send later.
	//
	// If native === true, the spec is resolved with native symbols (vs client symbols).
	//
	// See AttrSpec docs.
	ResolveAttrSpec(attrSpec string, native bool) (AttrUID, error)

	// Instantiates an attr element value for an AttrID -- typically followed by AttrElemVal.Unmarshal()
	NewAttrElem(attrDefID uint32, native bool) (AttrElemVal, error)
}

// NewRegistry returns a new Registry
func NewRegistry() Registry {
	return newRegistry()
}

// PinContext wraps a client request to receive a cell's state / updates.
type PinContext interface {
	task.Context // Started as a CHILD of the arc.PinnedCell returned by AppInstance.PinCell()

	PinReq // Originating request info

	// Marshals a CellOp and optional value to the given Tx's data store.
	//
	// If the given attr is not enabled within this PinContext, this function is a no-op.
	MarshalCellOp(dst *TxMsg, op CellOp, val AttrElemVal)

	// PushTx pushes the given tx to the originator of this PinContext.
	PushTx(tx *TxMsg) error

	// App returns the resolved AppContext that is servicing this PinContext
	App() AppContext
}

// PinReq is support wrapper for PinRequest, a client request to pin a cell.
type PinReq interface {
	Params() *PinReqParams
	URLPath() []string
}

// PinReqParams implements PinReq
type PinReqParams struct {
	PinReq   PinRequest
	PinCell  CellID
	URL      *url.URL
	ReqID    uint64        // Request ID needed to route to the originator
	LogLabel string        // info string for logging and debugging
	Outlet   chan *TxMsg // send to this channel to transmit to the request originator

}
