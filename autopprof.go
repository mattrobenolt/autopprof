package autopprof

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"time"
)

type CheckFunc func(*Config) bool

type Config struct {
	Interval   time.Duration
	Directory  string
	Check      CheckFunc
	ErrorLog   func(error)
	MaxRecords int

	records int
	ctx     context.Context
	cancel  context.CancelFunc
}

func (c *Config) Start(ctx context.Context) error {
	if c.cancel != nil {
		return nil
	}
	if err := os.MkdirAll(c.Directory, 0o700); err != nil {
		return err
	}
	c.ctx, c.cancel = context.WithCancel(context.Background())
	defer c.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-c.ctx.Done():
			return nil
		case <-time.After(c.Interval):
			if c.Check(c) {
				if err := c.writeProfile(); err != nil {
					if c.ErrorLog != nil {
						c.ErrorLog(err)
					}
				} else {
					c.records++
					if c.records > c.MaxRecords {
						if err := c.cleanup(); err != nil && c.ErrorLog != nil {
							c.ErrorLog(err)
						}
					}
				}
			}
		}
	}
}

func (c *Config) Stop() {
	if c.cancel != nil {
		c.cancel()
		c.ctx = nil
		c.cancel = nil
	}
}

func (c *Config) writeProfile() error {
	fname := fmt.Sprintf("heap_%d.pprof", time.Now().UnixNano())
	fp, err := os.Create(filepath.Join(c.Directory, fname))
	if err != nil {
		return err
	}
	defer fp.Close()
	return pprof.WriteHeapProfile(fp)
}

func (c *Config) cleanup() error {
	ds, err := os.ReadDir(c.Directory)
	if err != nil {
		return err
	}

	if len(ds) < c.MaxRecords {
		return nil
	}

	for _, d := range ds[0 : len(ds)-c.MaxRecords] {
		_ = os.Remove(filepath.Join(c.Directory, d.Name()))
	}
	return nil
}

func MemoryLimit(limit uint64) CheckFunc {
	memStats := new(runtime.MemStats)
	return func(_ *Config) bool {
		runtime.ReadMemStats(memStats)
		over := memStats.HeapAlloc > limit
		return over
	}
}

func (c *Config) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		switch r.URL.Path {
		case "/":
			ds, err := os.ReadDir(c.Directory)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			for _, d := range ds {
				io.WriteString(w, d.Name()+"\n")
			}
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		case "/start":
			go c.Start(context.Background())
			w.WriteHeader(201)
		case "/stop":
			c.Stop()
			w.WriteHeader(201)
		case "/latest":
			ds, err := os.ReadDir(c.Directory)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			if len(ds) == 0 {
				http.Error(w, "no profiles", 404)
				return
			}
			prof := ds[len(ds)-1]
			fp, err := os.Open(filepath.Join(c.Directory, prof.Name()))
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			defer fp.Close()
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, prof.Name()))
			io.Copy(w, fp)
		}
	})
}
