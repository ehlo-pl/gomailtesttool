//go:build !integration

package content

import (
	"context"
	"io"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ehlo-pl/gomailtesttool/internal/common/wikicontent"
)

func TestWebNext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `[{"title":"One","author":"A","lines":["x","y"]},{"title":"Two","author":"A","lines":["z"]}]`)
	}))
	defer srv.Close()
	web := Web{NewFetcher: func(categories []string) *wikicontent.Fetcher {
		f := wikicontent.NewFetcher(categories)
		f.PoetryURL, f.Client = srv.URL, srv.Client()
		return f
	}}
	rng := rand.New(rand.NewPCG(1, 2))
	next := func(used map[string]bool) (Item, error) {
		return web.Next(context.Background(), []string{"poetry"}, nil, used, rng)
	}
	item, err := next(map[string]bool{"poetry:One by A": true})
	if err != nil || item.Key != "poetry:Two by A" || item.Subject != "Two by A" || item.Text != "z" {
		t.Fatalf("Next() = %+v, %v; want the unused poem", item, err)
	}
	if _, err := next(map[string]bool{"poetry:One by A": true, "poetry:Two by A": true}); err == nil {
		t.Error("Next() repeated content instead of failing")
	}
}
