package selfupdate

var log Logger = &emptyLogger{}

// SetLogger redirects all logs to the logger defined in parameter.
// By default logs are not sent anywhere.
func SetLogger(logger Logger) {
	log = logger
}

// Logger interface. Compatible with standard log.Logger
type Logger interface {
	// Print calls Output to print to the standard logger. Arguments are handled in the manner of fmt.Print.
	Print(v ...interface{})
	// Printf calls Output to print to the standard logger. Arguments are handled in the manner of fmt.Printf.
	Printf(format string, v ...interface{})
}

// emptyLogger to discard all logs by default
type emptyLogger struct{}

func (l *emptyLogger) Print(v ...interface{})                 {}
func (l *emptyLogger) Printf(format string, v ...interface{}) {}
