package discover

import (
	"context"
	"crypto/x509"
	"net/http"
	"os"

	"github.com/bmc-toolbox/bmclib/errors"
	"github.com/bmc-toolbox/bmclib/internal/httpclient"
	"github.com/bmc-toolbox/bmclib/logging"

	"github.com/bmc-toolbox/bmclib/providers/dummy/ibmc"
	"github.com/go-logr/logr"
)

const (
	ProbeHpIlo         = "hpilo"
	ProbeIdrac8        = "idrac8"
	ProbeIdrac9        = "idrac9"
	ProbeSupermicrox   = "supermicrox"
	ProbeSupermicrox11 = "supermicrox11"
	ProbeHpC7000       = "hpc7000"
	ProbeM1000e        = "m1000e"
	ProbeQuanta        = "quanta"
	ProbeHpCl100       = "hpcl100"
)

// ScanAndConnect will scan the BMC trying to deduce the device type and return a working connection.
func ScanAndConnect(host string, username string, password string, options ...Option) (bmcConnection interface{}, err error) {
	opts := &Options{HintCallback: func(_ string) error { return nil }}
	for _, optFn := range options {
		optFn(opts)
	}
	if opts.Logger.GetSink() == nil {
		// create a default logger
		opts.Logger = logging.DefaultLogger()
	}
	if opts.Context == nil {
		opts.Context = context.Background()
	}
	opts.Logger.V(1).Info("detecting vendor", "step", "ScanAndConnect", "host", host)

	// return a connection to our dummy device.
	if os.Getenv("BMCLIB_TEST") == "1" {
		opts.Logger.V(1).Info("returning connection to dummy ibmc device.", "step", "ScanAndConnect", "host", host)
		bmc, err := ibmc.New(host, username, password)
		return bmc, err
	}

	client, err := httpclient.Build(opts.httpClientSetupFuncs...)
	if err != nil {
		return nil, err
	}

	probe := Probe{client: client, username: username, password: password, host: host, secureTLS: opts.secureTLS}

	devices := map[string]func(context.Context, logr.Logger) (interface{}, error){
		ProbeHpIlo:         probe.hpIlo,
		ProbeIdrac8:        probe.idrac8,
		ProbeIdrac9:        probe.idrac9,
		ProbeSupermicrox11: probe.supermicrox11,
		ProbeSupermicrox:   probe.supermicrox,
		ProbeHpC7000:       probe.hpC7000,
		ProbeM1000e:        probe.m1000e,
		ProbeQuanta:        probe.quanta,
		ProbeHpCl100:       probe.hpCl100,
	}

	order := []string{
		ProbeHpIlo,
		ProbeIdrac8,
		ProbeIdrac9,
		ProbeSupermicrox11,
		ProbeSupermicrox,
		ProbeHpC7000,
		ProbeM1000e,
		ProbeQuanta,
		ProbeHpCl100,
	}

	if opts.Hint != "" {
		swapProbe(order, opts.Hint)
	}

	for _, probeID := range order {
		probeDevice := devices[probeID]

		opts.Logger.V(1).Info("probing to identify device", "step", "ScanAndConnect", "host", host, "vendor", probeID)

		bmcConnection, err := probeDevice(opts.Context, opts.Logger)
		// if the device didn't match continue to probe
		if err != nil {
			// log error if probe is not successful
			opts.Logger.V(1).Info("Probe failed!",
				"step", "ScanAndConnect",
				"host", host,
				"vendor", probeID,
				"Error", err,
			)
			continue
		}
		if hintErr := opts.HintCallback(probeID); hintErr != nil {
			return nil, hintErr
		}
		// return a bmcConnection
		if bmcConnection != nil {
			return bmcConnection, nil
		}
	}

	return nil, errors.ErrVendorUnknown
}

// Options to pass in
type Options struct {
	// Hint is a probe ID that hints which probe should be probed first.
	Hint string

	// HintCallBack is a function that is called back with a probe ID that might be used
	// for the next ScanAndConnect attempt.  The callback is called only on successful scan.
	// If your code persists the hint as "best effort", always return a nil error.  Callback is
	// synchronous.
	HintCallback func(string) error
	Logger       logr.Logger
	Context      context.Context

	secureTLS            bool
	certPool             *x509.CertPool
	httpClientSetupFuncs []func(*http.Client)
}

// Option is part of the functional options pattern, see the `With*` functions and
// https://dave.cheney.net/2014/10/17/functional-options-for-friendly-apis
type Option func(*Options)

// WithSecureTLS enforces trusted TLS connections, with an optional CA certificate pool.
// Using this option with an nil pool uses the system CAs.
func WithSecureTLS(rootCAs *x509.CertPool) Option {
	return func(arg *Options) {
		// this function modifies an *http.Client's transport to verify server certs
		arg.httpClientSetupFuncs = append(arg.httpClientSetupFuncs, func(c *http.Client) {
			httpclient.SecureTLS(c, rootCAs)
			arg.secureTLS = true
			arg.certPool = rootCAs
		})
	}
}

// WithProbeHint sets the Options.Hint option.
func WithProbeHint(hint string) Option { return func(args *Options) { args.Hint = hint } }

// WithHintCallBack sets the Options.HintCallback option.
func WithHintCallBack(fn func(string) error) Option {
	return func(args *Options) { args.HintCallback = fn }
}

// WithLogger sets the Options.Logger option
func WithLogger(log logr.Logger) Option { return func(args *Options) { args.Logger = log } }

// WithContext sets the Options.Context option
func WithContext(ctx context.Context) Option { return func(args *Options) { args.Context = ctx } }

func swapProbe(order []string, hint string) {
	// With so few elements and since `==` uses SIMD,
	// looping is faster than having yet another hash map.
	for i := range order {
		if order[i] == hint {
			order[0], order[i] = order[i], order[0]
			break
		}
	}
}
