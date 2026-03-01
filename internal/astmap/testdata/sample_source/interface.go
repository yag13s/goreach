package sample

// Stringer is an interface with bodyless method declarations.
type Stringer interface {
	String() string
}

// Runner is an interface with multiple bodyless methods.
type Runner interface {
	Run() error
	Stop()
}

// ConcreteRunner implements Runner.
type ConcreteRunner struct{}

func (c *ConcreteRunner) Run() error {
	return nil
}

func (c *ConcreteRunner) Stop() {}
