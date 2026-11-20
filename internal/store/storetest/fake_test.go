package storetest_test

import (
	"testing"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/store/storetest"
)

func TestFakeKeepsTheStoreContract(t *testing.T) {
	storetest.Run(t, func(*testing.T) storetest.Subject {
		fake := storetest.NewFake()
		return storetest.Subject{
			Store:          fake,
			AddUser:        func(_ *testing.T, user store.User) store.User { return fake.AddUser(user) },
			AddCustomer:    func(_ *testing.T, c store.Customer) store.Customer { return fake.AddCustomer(c) },
			AddProduct:     func(_ *testing.T, product store.Product) store.Product { return fake.AddProduct(product) },
			AddOnlineOrder: func(_ *testing.T, seed storetest.OnlineOrderSeed) int64 { return fake.AddOnlineOrder(seed) },
		}
	})
}
