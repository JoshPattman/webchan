// package webchan provides a simple wrapper for a web socket which enables sending of typed data over the socket using json serialization.
// The WebChan type communicates with your code exclusively using channels, and is completely thread safe.
package webchan

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"reflect"
)

// WebChan is a wrapper for a web socket which enables sending of typed data over the socket.
// Sending and receiving data is done through the Send and Recv channels, and errors are sent through the Error channel.
// This means that WebChan is completely thread safe, and can be used in a concurrent environment.
// WebChan takes the error handling approach of logging errors to the error channel and continuing operations,
// with the exception of io.EOF and net.ErrClosed errors, which will close the Recv channel and stop reading from the socket.
// The Close() method can be used to shutdown the WebChan and it's underlying socket.
type WebChan struct {
	// The send channel to send data over the socket.
	// Pushing data to this channel will send the data at some point in the future (it is non instantanious, but order will be preserved).
	// You can only push data of the types specified in the constructor, any other types will result in a panic.
	// Any errors resulting from ther sending will be pushed to the Error channel.
	Send chan interface{}
	// The recv channel to receive data from the socket.
	// This channel will become closed when either Close() is called or the socket is closed with an io.EOF or net.ErrClosed error.
	// Other errors will be pushed to the Error channel.
	// The data in this channel is typed but wrapped as an interface{}, so a type switch may be required to determine the type.
	Recv chan interface{}
	// The error channel to receive errors from the socket operations.
	// This channel will never be closed.
	// Once this channel fills with errors, it will start dropping errors.
	Error         chan error
	closeRecvChan chan struct{}
	soc           net.Conn
	allowedTypes  map[string]reflect.Type
	encoder       *json.Encoder
	decoder       *json.Decoder
}

// NewWebChan creates a new WebChan with the given socket, buffer length and allowed types.
// The buffer length is the maximum number of messages that can be queued for sending or receiving.
// The allowed types are a set of example types that can be sent over the socket, any other type will cause a panic (on send) or an error (on recv).
// IMPORTANT: All types must be json serializable.
func NewWebChan(soc net.Conn, bufLength int, allowedTypes ...interface{}) *WebChan {
	allowedTypesMap := make(map[string]interface{}, len(allowedTypes))
	for _, v := range allowedTypes {
		allowedTypesMap[fmt.Sprintf("%T", v)] = v
	}
	return NewNamedWebChan(soc, bufLength, allowedTypesMap)
}

// NewNamedWebChan creates a new WebChan with the given socket, buffer length and allowed types.
// The buffer length is the maximum number of messages that can be queued for sending or receiving.
// The allowed types are a named set of example types that can be sent over the socket, any other type will cause a panic (on send) or an error (on recv).
// Names must be unique.
// IMPORTANT: All types must be json serializable.
func NewNamedWebChan(soc net.Conn, bufLength int, allowedTypes map[string]interface{}) *WebChan {
	allowedTypesReflect := make(map[string]reflect.Type, len(allowedTypes))
	for k, v := range allowedTypes {
		allowedTypesReflect[k] = reflect.TypeOf(v)
	}
	wc := &WebChan{
		make(chan interface{}, bufLength),
		make(chan interface{}, bufLength),
		make(chan error, 100),
		make(chan struct{}),
		soc,
		allowedTypesReflect,
		json.NewEncoder(soc),
		json.NewDecoder(soc),
	}

	// Send routine
	go func() {
		for {
			// To close this goroutine, we should close the Send channel
			data, ok := <-wc.Send
			if !ok {
				return
			}
			typeName, ok := wc.getAllowedTypeName(data)
			if !ok {
				panic(&sendError{fmt.Errorf("type not allowed: %T", data)})
			}
			err := wc.encoder.Encode(typeName)
			if err == nil {
				err = wc.encoder.Encode(data)
			}
			if err != nil {
				wc.tryPushError(&sendError{err})
				continue
			}
		}
	}()
	// Recv routine
	go func() {
		defer func() {
			close(wc.Recv)
		}()
		for {
			select {
			case <-wc.closeRecvChan:
				return
			default:
				var typeName string
				err := wc.decoder.Decode(&typeName)
				if err != nil {
					if errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF) {
						return
					} else {
						wc.tryPushError(&recvError{err})
					}
					continue
				}
				data, ok := wc.createTypeInterfaceReflectPointer(typeName)
				if !ok {
					wc.tryPushError(&recvError{fmt.Errorf("type not allowed: %s", typeName)})
					continue
				}
				err = wc.decoder.Decode(data.Interface())
				if err != nil {
					if errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF) {
						return
					} else {
						wc.tryPushError(&recvError{err})
					}
					continue
				}
				wc.Recv <- data.Elem().Interface()
			}

		}
	}()
	return wc
}

// Close does not immidiately stop all processing, but will close both the send and recv channel, and soon stop reading the incoming socket and close it too.
// Close will only do somthing on the first time it is called, subsequent calls will do nothing.
func (wc *WebChan) Close() {
	select {
	case _, ok := <-wc.closeRecvChan:
		if !ok {
			// Already closed
			return
		}
	default:
		close(wc.closeRecvChan)
		close(wc.Send)
		wc.soc.Close() // This will cause blocking operations on the socket to error out of blocking
	}
}

func (wc *WebChan) getAllowedTypeName(data interface{}) (string, bool) {
	t := reflect.TypeOf(data)
	for k, v := range wc.allowedTypes {
		if v == t {
			return k, true
		}
	}
	return "", false
}

func (wc *WebChan) createTypeInterfaceReflectPointer(typeName string) (reflect.Value, bool) {
	t, ok := wc.allowedTypes[typeName]
	if !ok {
		return reflect.Value{}, false
	}
	return reflect.New(t), true
}

func (wc *WebChan) tryPushError(err error) {
	select {
	case wc.Error <- err:
	default:
		fmt.Println("Error channel full, should probably check it more frequently")
	}
}
