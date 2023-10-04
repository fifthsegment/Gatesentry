package gatesentryf

import (
	"fmt"
	"log"

	"github.com/kardianos/service"
)

type program struct {
	exit chan struct{}
}

//prog(state) runs in a child process

type fn func()

var RootFn fn

func SVCAddRoot(fnc fn) {
	RootFn = fnc
}

// Service setup.
//   Define service config.
//   Create the service.
//   Setup the logger.
//   Handle service controls (optional).
//   Run the service.

func RunGateSentryServiceRunner(svcFlag string) {
	svcConfig := &service.Config{
		Name:        "GateSentry",
		DisplayName: "A web filtering proxy",
		Description: "A web filtering proxy",
	}

	prg := &program{}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		log.Fatal(err)
	}
	errs := make(chan error, 5)
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		for {
			err := <-errs
			if err != nil {
				log.Print(err)
			}
		}
	}()

	if len(svcFlag) != 0 {
		err := service.Control(s, svcFlag)
		if err != nil {
			log.Printf("Valid actions: %q\n", service.ControlAction)
			log.Fatal(err)
		}
		return
	}
	err = s.Run()
	if err != nil {
		// logger.Error(err)
	}
}

func (p *program) Start(s service.Service) error {
	log.Println("Starting up GateSentry")
	p.exit = make(chan struct{})
	go p.run()
	return nil
}

func (p *program) run() error {
	RootFn()
	for {
		select {

		case <-p.exit:
			// fmt.Println("Stop signal received")
			// log.Println("Stopping GateSentry")
			Stop()
			return nil
		}
	}
}
func (p *program) Stop(s service.Service) error {
	fmt.Println("Stopping GateSentry")
	Stop()
	close(p.exit)
	return nil
}
