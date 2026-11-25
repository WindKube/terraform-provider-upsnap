package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

const defaultPerPage = 200

// Collection is a typed handle on a PocketBase record collection.
type Collection[T any] struct {
	c    *Client
	name string
}

type ListOptions struct {
	Filter  string
	Sort    string
	PerPage int
}

type listPage[T any] struct {
	Page       int `json:"page"`
	PerPage    int `json:"perPage"`
	TotalItems int `json:"totalItems"`
	TotalPages int `json:"totalPages"`
	Items      []T `json:"items"`
}

var ErrNotUnique = errors.New("more than one record matches")

func (s Collection[T]) Name() string { return s.name }

func (s Collection[T]) path(id string) string {
	p := "/api/collections/" + url.PathEscape(s.name) + "/records"
	if id != "" {
		p += "/" + url.PathEscape(id)
	}
	return p
}

func (s Collection[T]) Get(ctx context.Context, id string) (*T, error) {
	if id == "" {
		return nil, fmt.Errorf("%s: empty record id", s.name)
	}
	var out T
	if err := s.c.do(ctx, http.MethodGet, s.path(id), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// List returns every record matching opts, following pagination until the last page.
func (s Collection[T]) List(ctx context.Context, opts ListOptions) ([]T, error) {
	perPage := opts.PerPage
	if perPage <= 0 {
		perPage = defaultPerPage
	}
	var all []T
	for page := 1; ; page++ {
		q := url.Values{}
		q.Set("page", strconv.Itoa(page))
		q.Set("perPage", strconv.Itoa(perPage))
		if opts.Filter != "" {
			q.Set("filter", opts.Filter)
		}
		if opts.Sort != "" {
			q.Set("sort", opts.Sort)
		}
		var resp listPage[T]
		if err := s.c.do(ctx, http.MethodGet, s.path(""), q, nil, &resp); err != nil {
			return nil, err
		}
		all = append(all, resp.Items...)
		if page >= resp.TotalPages || len(resp.Items) == 0 {
			return all, nil
		}
	}
}

// FindOne returns the only record matching filter, a 404 APIError when none match and
// ErrNotUnique when several do.
func (s Collection[T]) FindOne(ctx context.Context, filter string) (*T, error) {
	items, err := s.List(ctx, ListOptions{Filter: filter, PerPage: 2})
	if err != nil {
		return nil, err
	}
	switch len(items) {
	case 0:
		return nil, &APIError{Method: http.MethodGet, Path: s.path(""), StatusCode: http.StatusNotFound, Message: fmt.Sprintf("no %s record matches %s", s.name, filter)}
	case 1:
		return &items[0], nil
	default:
		return nil, fmt.Errorf("%w in %s for filter %s", ErrNotUnique, s.name, filter)
	}
}

func (s Collection[T]) Create(ctx context.Context, body any) (*T, error) {
	var out T
	if err := s.c.do(ctx, http.MethodPost, s.path(""), nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (s Collection[T]) Update(ctx context.Context, id string, body any) (*T, error) {
	var out T
	if err := s.c.do(ctx, http.MethodPatch, s.path(id), nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (s Collection[T]) Delete(ctx context.Context, id string) error {
	return s.c.do(ctx, http.MethodDelete, s.path(id), nil, nil, nil)
}
