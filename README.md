# `webchan` - A simple package to make handling socket communications easier

Webchan wraps a socket, allowing it to be communicated through using channels. It also automatically sends type information with each message, allowing for easy type checking using type switch statements. The aim of webchan is to maximise te use of go's concurrency and type features, enabling a simpler and more robust way to communicate between go programs.

Webchan sends all data using json serialisation, so it is imperative that the structs which are sent are serialisable by the json package.

## Example
```go
// Create a pair of connected sockets
a, b := net.Pipe()
// Create the WebChan objects.
// Note that after this point, the WebChan objects are responsible for closing the sockets, and the socets should not be acsessed directly.
// We tell both webchans that they are allowed to send strings and testType objects.
wca, wcb := NewWebChan(a, 100, "strings", testType{}), NewWebChan(b, 100, "strings", testType{})
// Ensure that the sockets are closed when the test ends
defer wca.Close()
defer wcb.Close()

// Send a string from wca to wcb (this is non blocking, so all we know is the receiver will probably get the message int he future)
wca.Send <- "Hello"
// Send a testType from wca to wcb (this program will never actually read this data, but it is here for example purposes)
wca.Send <- testType{A: 42, B: "Hello"}

// Receive the data from wca to wcb, or an error if one exists, or a timeout if the message is not received in time.
select {
// A message (data) has been received and decoded sucsessfully
case msg := <-wcb.Recv:
    // We now have the data as an interface{}, so we need to type switch to get the actual data
    switch msg := msg.(type) {
    case string:
        fmt.Println("Received string:", msg)
    case testType:
        fmt.Println("Received testType with A =", msg.A, "and B =", msg.B)
    default:
        t.Errorf("Received unknown type (this should never happen in this program): %T", msg)
    }
// wcb has encountered an error
case err := <-wcb.Error:
    t.Errorf("Error with WebChan b: %s", err)
// wca has encountered an error
case err := <-wca.Error:
    t.Errorf("Error with WebChan a: %s", err)
// A timeout has occured (this should never happen in this program as the communication is instant)
case <-time.After(time.Second * 2):
    t.Errorf("Timeout waiting for message")
}
```