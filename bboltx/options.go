package bboltx

import (
	"log/slog"

	"github.com/arcgolabs/storx/observer"
)

type BucketOption func(*bucketOptions)

type bucketOptions struct {
	createBucketIfMissing  bool
	readOnlyMissingAsEmpty bool
	logger                 *slog.Logger
	observers              []observer.Observer
}

func defaultBucketOptions() bucketOptions {
	return bucketOptions{
		createBucketIfMissing:  true,
		readOnlyMissingAsEmpty: true,
	}
}

// WithCreateBucketIfMissing controls whether write transactions create the
// target bucket automatically.
func WithCreateBucketIfMissing(enabled bool) BucketOption {
	return func(opts *bucketOptions) {
		opts.createBucketIfMissing = enabled
	}
}

// WithReadOnlyMissingAsEmpty controls whether missing buckets behave like empty
// buckets in read-only operations.
func WithReadOnlyMissingAsEmpty(enabled bool) BucketOption {
	return func(opts *bucketOptions) {
		opts.readOnlyMissingAsEmpty = enabled
	}
}

// WithLogger overrides the logger for this bucket wrapper.
func WithLogger(logger *slog.Logger) BucketOption {
	return func(opts *bucketOptions) {
		opts.logger = logger
	}
}

// WithObserver adds a bucket-level observer.
func WithObserver(obs observer.Observer) BucketOption {
	return func(opts *bucketOptions) {
		if obs != nil {
			opts.observers = append(opts.observers, obs)
		}
	}
}

// WithObservers adds bucket-level observers.
func WithObservers(observers ...observer.Observer) BucketOption {
	return func(opts *bucketOptions) {
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
