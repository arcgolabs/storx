package badgerx

import (
	"log/slog"
	"time"

	"github.com/arcgolabs/storx/observer"
)

type NamespaceOption func(*namespaceOptions)

type namespaceOptions struct {
	prefixSeparator string
	copyValue       bool
	logger          *slog.Logger
	observers       []observer.Observer
}

func defaultNamespaceOptions() namespaceOptions {
	return namespaceOptions{
		prefixSeparator: "/",
		copyValue:       true,
	}
}

// WithPrefixSeparator appends the separator when the namespace prefix is
// non-empty and does not already end with it.
func WithPrefixSeparator(separator string) NamespaceOption {
	return func(opts *namespaceOptions) {
		opts.prefixSeparator = separator
	}
}

// WithCopyValue controls whether values are copied before decoding.
func WithCopyValue(enabled bool) NamespaceOption {
	return func(opts *namespaceOptions) {
		opts.copyValue = enabled
	}
}

// WithLogger overrides the logger for this namespace wrapper.
func WithLogger(logger *slog.Logger) NamespaceOption {
	return func(opts *namespaceOptions) {
		opts.logger = logger
	}
}

// WithObserver adds a namespace-level observer.
func WithObserver(obs observer.Observer) NamespaceOption {
	return func(opts *namespaceOptions) {
		if obs != nil {
			opts.observers = append(opts.observers, obs)
		}
	}
}

// WithObservers adds namespace-level observers.
func WithObservers(observers ...observer.Observer) NamespaceOption {
	return func(opts *namespaceOptions) {
		for _, obs := range observers {
			if obs != nil {
				opts.observers = append(opts.observers, obs)
			}
		}
	}
}

type DBOption func(*dbOptions)

type dbOptions struct {
	logger    *slog.Logger
	observers []observer.Observer
}

// WithDBLogger sets the DB wrapper logger.
func WithDBLogger(logger *slog.Logger) DBOption {
	return func(opts *dbOptions) {
		opts.logger = logger
	}
}

// WithDBObserver adds a DB-level observer.
func WithDBObserver(obs observer.Observer) DBOption {
	return func(opts *dbOptions) {
		if obs != nil {
			opts.observers = append(opts.observers, obs)
		}
	}
}

// WithDBObservers adds DB-level observers.
func WithDBObservers(observers ...observer.Observer) DBOption {
	return func(opts *dbOptions) {
		for _, obs := range observers {
			if obs != nil {
				opts.observers = append(opts.observers, obs)
			}
		}
	}
}

type SetOption func(*setOptions)

type setOptions struct {
	hasTTL  bool
	ttl     time.Duration
	hasMeta bool
	meta    byte
	discard bool
}

// WithTTL sets a Badger TTL for the entry being written.
func WithTTL(ttl time.Duration) SetOption {
	return func(opts *setOptions) {
		opts.hasTTL = true
		opts.ttl = ttl
	}
}

// WithMeta sets the Badger user meta byte for the entry being written.
func WithMeta(meta byte) SetOption {
	return func(opts *setOptions) {
		opts.hasMeta = true
		opts.meta = meta
	}
}

// WithDiscard sets the discard flag on the Badger entry.
func WithDiscard() SetOption {
	return func(opts *setOptions) {
		opts.discard = true
	}
}
