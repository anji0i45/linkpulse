package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/graphql-go/graphql"
	"github.com/graphql-go/handler"

	"linkpulse/internal/store"
)

// newGraphQLHandler exposes the same local analytics data through GraphQL.
// It uses the same store as the REST API; no extra database is needed.
func newGraphQLHandler(st store.Store) http.Handler {
	linkType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Link",
		Fields: graphql.Fields{
			"code": &graphql.Field{
				Type: graphql.NewNonNull(graphql.String),
				Resolve: func(p graphql.ResolveParams) (any, error) {
					return p.Source.(store.LinkSummary).Code, nil
				},
			},
			"longUrl": &graphql.Field{
				Type: graphql.NewNonNull(graphql.String),
				Resolve: func(p graphql.ResolveParams) (any, error) {
					return p.Source.(store.LinkSummary).LongURL, nil
				},
			},
			"createdAt": &graphql.Field{
				Type: graphql.NewNonNull(graphql.String),
				Resolve: func(p graphql.ResolveParams) (any, error) {
					return p.Source.(store.LinkSummary).CreatedAt.Format(time.RFC3339), nil
				},
			},
			"clicks": &graphql.Field{
				Type: graphql.NewNonNull(graphql.Int),
				Resolve: func(p graphql.ResolveParams) (any, error) {
					return p.Source.(store.LinkSummary).Clicks, nil
				},
			},
			"disabled": &graphql.Field{
				Type: graphql.NewNonNull(graphql.Boolean),
				Resolve: func(p graphql.ResolveParams) (any, error) {
					return p.Source.(store.LinkSummary).Disabled, nil
				},
			},
			"expiresAt": &graphql.Field{
				Type: graphql.String,
				Resolve: func(p graphql.ResolveParams) (any, error) {
					link := p.Source.(store.LinkSummary)
					if link.ExpiresAt == nil {
						return nil, nil
					}
					return link.ExpiresAt.Format(time.RFC3339), nil
				},
			},
		},
	})

	dayType := graphql.NewObject(graphql.ObjectConfig{
		Name: "DayCount",
		Fields: graphql.Fields{
			"day": &graphql.Field{
				Type: graphql.NewNonNull(graphql.String),
				Resolve: func(p graphql.ResolveParams) (any, error) {
					return p.Source.(store.DayCount).Day, nil
				},
			},
			"clicks": &graphql.Field{
				Type: graphql.NewNonNull(graphql.Int),
				Resolve: func(p graphql.ResolveParams) (any, error) {
					return p.Source.(store.DayCount).Clicks, nil
				},
			},
		},
	})

	referrerType := graphql.NewObject(graphql.ObjectConfig{
		Name: "ReferrerCount",
		Fields: graphql.Fields{
			"name": &graphql.Field{
				Type: graphql.NewNonNull(graphql.String),
				Resolve: func(p graphql.ResolveParams) (any, error) {
					return p.Source.(store.NameCount).Name, nil
				},
			},
			"clicks": &graphql.Field{
				Type: graphql.NewNonNull(graphql.Int),
				Resolve: func(p graphql.ResolveParams) (any, error) {
					return p.Source.(store.NameCount).Clicks, nil
				},
			},
		},
	})

	analyticsType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Analytics",
		Fields: graphql.Fields{
			"code": &graphql.Field{
				Type: graphql.NewNonNull(graphql.String),
				Resolve: func(p graphql.ResolveParams) (any, error) {
					return p.Source.(store.Analytics).Code, nil
				},
			},
			"longUrl": &graphql.Field{
				Type: graphql.NewNonNull(graphql.String),
				Resolve: func(p graphql.ResolveParams) (any, error) {
					return p.Source.(store.Analytics).LongURL, nil
				},
			},
			"totalClicks": &graphql.Field{
				Type: graphql.NewNonNull(graphql.Int),
				Resolve: func(p graphql.ResolveParams) (any, error) {
					return p.Source.(store.Analytics).TotalClicks, nil
				},
			},
			"uniqueClicks": &graphql.Field{
				Type: graphql.NewNonNull(graphql.Int),
				Resolve: func(p graphql.ResolveParams) (any, error) {
					return p.Source.(store.Analytics).UniqueClicks, nil
				},
			},
			"clicksByDay": &graphql.Field{
				Type: graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(dayType))),
				Resolve: func(p graphql.ResolveParams) (any, error) {
					return p.Source.(store.Analytics).ClicksByDay, nil
				},
			},
			"topReferrers": &graphql.Field{
				Type: graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(referrerType))),
				Resolve: func(p graphql.ResolveParams) (any, error) {
					return p.Source.(store.Analytics).TopReferrers, nil
				},
			},
		},
	})

	queryType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Query",
		Fields: graphql.Fields{
			"links": &graphql.Field{
				Type: graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(linkType))),
				Resolve: func(p graphql.ResolveParams) (any, error) {
					return st.ListLinks(p.Context)
				},
			},
			"analytics": &graphql.Field{
				Type: analyticsType,
				Args: graphql.FieldConfigArgument{
					"code": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
				},
				Resolve: func(p graphql.ResolveParams) (any, error) {
					code := p.Args["code"].(string)
					analytics, err := st.GetAnalytics(p.Context, code)
					if errors.Is(err, store.ErrNotFound) {
						return nil, nil
					}
					if err != nil {
						return nil, err
					}
					return analytics, nil
				},
			},
		},
	})

	schema, err := graphql.NewSchema(graphql.SchemaConfig{Query: queryType})
	if err != nil {
		panic("build graphql schema: " + err.Error())
	}

	return handler.New(&handler.Config{
		Schema:   &schema,
		Pretty:   true,
		GraphiQL: true,
	})
}
