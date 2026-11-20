package api

import (
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

// actor is who did something, in every /v2 operation that names one (business ADR 0014):
// a User, a Customer, or the system. The legacy operations name only Users.
type actor struct {
	Kind string `json:"kind" enum:"user,customer,system" doc:"A User in StockFlow, a Customer on the Storefront, or the system."`
	ID   *int64 `json:"id" doc:"The User's or Customer's id; null for the system, and for a Customer the API cannot name yet."`
	Name string `json:"name" doc:"The User's or Customer's name now; for the system, System."`
}

const systemActorName = "System"

// userOrSystemActor is the actor of a column that holds a User, or the system: a Stock
// movement's user_id and system flag, or an Order's cancelled_by and cancelled_by_system.
func userOrSystemActor(id pgtype.Int8, name pgtype.Text, system bool) actor {
	if system {
		return actor{Kind: store.ActorSystem, Name: systemActorName}
	}
	return userActor(id, name)
}

func userActor(id pgtype.Int8, name pgtype.Text) actor {
	return actor{Kind: store.ActorUser, ID: &id.Int64, Name: name.String}
}

// orderCreator is who created an Order (orders R1, R21): the User of an In-shop order, the
// Customer of an Online order. The Customer has no record yet (the Customers ticket adds
// it), so the Order's own customer name, which is theirs when it was placed (orders R22),
// stands in and the id is null.
func orderCreator(d store.ReadOrderRow) actor {
	if d.Channel == store.ChannelOnline {
		return actor{Kind: store.ActorCustomer, Name: d.Customer}
	}
	return userActor(d.CreatedBy, d.CreatedByName)
}
