package manager

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type Module interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

type Manager struct {
	modules         []Module
	shutdownTimeout time.Duration
}

var shutdownTimeoutReached error = errors.New("shutdown timeout reached")

func (m Manager) Start(ctx context.Context) error {
	errs := make(chan error, len(m.modules))
	wg := sync.WaitGroup{}

	for _, module := range m.modules {
		wg.Add(1)
		go func() {
			errs <- module.Start(ctx)
			wg.Done()
		}()
	}

	return getErrs(errs)
}

func (m Manager) Stop(ctx context.Context) error {
	shutdownCtx, cancel := context.WithTimeoutCause(ctx, m.shutdownTimeout, shutdownTimeoutReached)
	defer cancel()

	wg := sync.WaitGroup{}

	errs := make(chan error, len(m.modules))
	for _, module := range m.modules {
		wg.Add(1)
		go func() {
			errs <- module.Stop(shutdownCtx)
			wg.Done()
		}()
	}

	select {
	case <-shutdownCtx.Done():
	case <-waitFor(&wg):
	}

	close(errs)

	return getErrs(errs)
}

func waitFor(wg *sync.WaitGroup) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	return done
}

func getErrs(errsCh <-chan error) error {
	errs := []error{}
	for range errsCh {
		err := <-errsCh
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

func (m Manager) WaitOnSignal(ctx context.Context, ctxCancel func()) {
	sysKill := make(chan os.Signal, 1)

	signal.Notify(sysKill, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	select {
	case <-ctx.Done():
	case <-sysKill:
	}

	ctxCancel()
}
