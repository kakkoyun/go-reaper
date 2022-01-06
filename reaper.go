package reaper

/*  Note:  This is a *nix only implementation.  */

//  Prefer #include style directives.
import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
)

type Logger interface {
	Log(keyvals ...interface{}) error
}

type Config struct {
	Pid              int
	Options          int
	DisablePid1Check bool
	Debug            bool
	Logger           Logger
}

// Handle death of child (SIGCHLD) messages. Pushes the signal onto the
// notifications channel if there is a waiter.
func sigChildHandler(ctx context.Context, notifications chan os.Signal) {
	var sigs = make(chan os.Signal, 3)
	signal.Notify(sigs, syscall.SIGCHLD)

	for {
		var sig = <-sigs
		select {
		case <-ctx.Done():
			return
		case notifications <- sig: /*  published it.  */
		default:
			/*
			 *  Notifications channel full - drop it to the
			 *  floor. This ensures we don't fill up the SIGCHLD
			 *  queue. The reaper just waits for any child
			 *  process (pid=-1), so we ain't loosing it!! ;^)
			 */
		}
	}

} /*  End of function  sigChildHandler.  */

// Be a good parent - clean up behind the children.
func reapChildren(ctx context.Context, config Config) error {
	logger := config.Logger
	var notifications = make(chan os.Signal, 1)

	go sigChildHandler(ctx, notifications)

	pid := config.Pid
	opts := config.Options

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case sig := <-notifications:
			level.Debug(logger).Log("msg", "received signal", "signal", sig)
		}

		for {
			var wstatus syscall.WaitStatus

			/*
			 *  Reap 'em, so that zombies don't accumulate.
			 *  Plants vs. Zombies!!
			 */
			pid, err := syscall.Wait4(pid, &wstatus, opts, nil)
			for syscall.EINTR == err {
				pid, err = syscall.Wait4(pid, &wstatus, opts, nil)
			}

			if syscall.ECHILD == err {
				break
			}
			level.Debug(logger).Log("msg", "clean up", "pid", pid, "wstatus", wstatus)
		}
	}
} /*   End of function  reapChildren.  */

/*
 *  ======================================================================
 *  Section: Exported functions
 *  ======================================================================
 */

// Reap Normal entry point for the reaper code. Start reaping children in the
// background inside a goroutine.
func Reap(ctx context.Context) error {
	/*
	 *  Only reap processes if we are taking over init's duties aka
	 *  we are running as pid 1 inside a docker container. The default
	 *  is to reap all processes.
	 */
	return Start(ctx, Config{
		Pid:              -1,
		Options:          0,
		DisablePid1Check: false,
	})
} /*  End of [exported] function  Reap.  */

// Start Entry point for invoking the reaper code with a specific configuration.
// The config allows you to bypass the pid 1 checks, so handle with care.
// The child processes are reaped in the background inside a goroutine.
func Start(ctx context.Context, config Config) error {
	if config.Logger == nil {
		var (
			logger log.Logger
			lvl    level.Option
		)
		if config.Debug {
			lvl = level.AllowDebug()
		} else {
			lvl = level.AllowInfo()
		}
		logger = log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
		logger = level.NewFilter(logger, lvl)
		logger = log.With(logger, "name", "grim-reaper")
		config.Logger = log.With(logger, "ts", log.DefaultTimestampUTC, "caller", log.DefaultCaller)
	}

	/*
	 *  Start the Reaper with configuration options. This allows you to
	 *  reap processes even if the current pid isn't running as pid 1.
	 *  So ... use with caution!!
	 *
	 *  In most cases, you are better off just using Reap() as that
	 *  checks if we are running as Pid 1.
	 */
	if !config.DisablePid1Check {
		mypid := os.Getpid()
		if 1 != mypid {
			return errors.New("grim reaper disabled, pid not 1")
		}
	}

	/*
	 *  Ok, so either pid 1 checks are disabled or we are the grandma
	 *  of 'em all, either way we get to play the grim reaper.
	 *  You will be missed, Terry Pratchett!! RIP
	 */
	return reapChildren(ctx, config)
} /*  End of [exported] function  Start.  */
