package storetest

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

func newProduct(name, sku string) store.InsertProductParams {
	return store.InsertProductParams{Name: name, Sku: sku, Unit: "pcs", Price: 4500}
}

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: true} }

func names(products []store.Product) []string {
	var got []string
	for _, p := range products {
		got = append(got, p.Name)
	}
	return got
}

// listProducts lists the Products that match arg on one page, Active ones unless arg says
// otherwise.
func listProducts(t *testing.T, s Subject, arg store.ListProductsParams) []store.Product {
	t.Helper()
	if arg.Status == "" {
		arg.Status = "active"
	}
	if arg.RowLimit == 0 {
		arg.RowLimit = 100
	}
	rows, err := s.Store.ListProducts(context.Background(), arg)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

// runProducts is the part of the contract that is Products (products R1-R14).
func runProducts(t *testing.T, newSubject func(t *testing.T) Subject) {
	ctx := context.Background()

	t.Run("creates an Active Product with stock 0", func(t *testing.T) {
		s := newSubject(t)

		got, err := s.Store.CreateProduct(ctx, store.InsertProductParams{
			Name: "Thai tea", Sku: "TEA-01", Unit: "box", Price: 12050, MinimumStock: 5,
		})

		if err != nil {
			t.Fatal(err)
		}
		want := store.Product{
			ID: got.ID, Name: "Thai tea", Sku: "TEA-01", Unit: "box", Price: 12050, MinimumStock: 5,
			Stock: 0, Status: "active", StockState: "out-of-stock",
		}
		if got != want {
			t.Errorf("got %+v, want %+v", got, want)
		}
		if got.ID == 0 {
			t.Error("the Product has no id")
		}
	})

	t.Run("gets a Product by id, and reports an unknown id as not found", func(t *testing.T) {
		s := newSubject(t)
		created, err := s.Store.CreateProduct(ctx, newProduct("Thai tea", "TEA-01"))
		if err != nil {
			t.Fatal(err)
		}

		got, err := s.Store.GetProduct(ctx, created.ID)
		if err != nil || got != created {
			t.Errorf("got %+v, %v; want %+v", got, err, created)
		}
		_, err = s.Store.GetProduct(ctx, created.ID+1)
		if !errors.Is(err, store.ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})

	// products R3.
	t.Run("refuses a SKU another Product has, whatever its letter case", func(t *testing.T) {
		s := newSubject(t)
		if _, err := s.Store.CreateProduct(ctx, newProduct("Thai tea", "Tea-01")); err != nil {
			t.Fatal(err)
		}

		_, err := s.Store.CreateProduct(ctx, newProduct("Green tea", "tEA-01"))

		if !errors.Is(err, store.ErrSKUTaken) {
			t.Errorf("err = %v, want ErrSKUTaken", err)
		}
		if got := listProducts(t, s, store.ListProductsParams{}); len(got) != 1 {
			t.Errorf("products = %+v, want only the first", got)
		}
	})

	// products R3: the SKU can change, and a Product can keep or recase its own.
	t.Run("changes a SKU unless another Product has it, in any letter case", func(t *testing.T) {
		s := newSubject(t)
		first, _ := s.Store.CreateProduct(ctx, newProduct("Thai tea", "TEA-01"))
		second, _ := s.Store.CreateProduct(ctx, newProduct("Green tea", "TEA-02"))

		_, err := s.Store.UpdateProduct(ctx, store.ChangeProductParams{ID: second.ID, Sku: text("tea-01")})
		if !errors.Is(err, store.ErrSKUTaken) {
			t.Errorf("taking another's SKU: err = %v, want ErrSKUTaken", err)
		}
		recased, err := s.Store.UpdateProduct(ctx, store.ChangeProductParams{ID: first.ID, Sku: text("tea-01")})
		if err != nil || recased.Sku != "tea-01" {
			t.Errorf("recasing its own SKU: got %+v, %v", recased, err)
		}
		moved, err := s.Store.UpdateProduct(ctx, store.ChangeProductParams{ID: second.ID, Sku: text("TEA-03")})
		if err != nil || moved.Sku != "TEA-03" {
			t.Errorf("changing the SKU: got %+v, %v", moved, err)
		}
		freed, err := s.Store.CreateProduct(ctx, newProduct("Black tea", "tea-02"))
		if err != nil {
			t.Errorf("the SKU TEA-02 was left free but: %v (%+v)", err, freed)
		}
	})

	t.Run("updates only the fields it is given, and never the stock", func(t *testing.T) {
		s := newSubject(t)
		seeded := s.AddProduct(t, store.Product{
			Name: "Thai tea", Sku: "TEA-01", Unit: "box", Price: 12050, MinimumStock: 5, Stock: 3, Status: "active",
		})

		got, err := s.Store.UpdateProduct(ctx, store.ChangeProductParams{
			ID: seeded.ID, Name: text("ชาไทย"), Price: pgtype.Int8{Int64: 0, Valid: true},
		})

		if err != nil {
			t.Fatal(err)
		}
		want := seeded
		want.Name, want.Price = "ชาไทย", 0
		if got != want {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})

	t.Run("updates every field, archives and restores", func(t *testing.T) {
		s := newSubject(t)
		created, _ := s.Store.CreateProduct(ctx, newProduct("Thai tea", "TEA-01"))

		got, err := s.Store.UpdateProduct(ctx, store.ChangeProductParams{
			ID: created.ID, Name: text("Green tea"), Sku: text("GRN"), Unit: text("pack"),
			Price: pgtype.Int8{Int64: 999999999, Valid: true}, MinimumStock: pgtype.Int4{Int32: 0, Valid: true},
			Status: text("archived"),
		})

		if err != nil {
			t.Fatal(err)
		}
		want := store.Product{
			ID: created.ID, Name: "Green tea", Sku: "GRN", Unit: "pack", Price: 999999999,
			Status: "archived", StockState: "out-of-stock",
		}
		if got != want {
			t.Errorf("got %+v, want %+v", got, want)
		}
		restored, err := s.Store.UpdateProduct(ctx, store.ChangeProductParams{ID: created.ID, Status: text("active")})
		if err != nil || restored.Status != "active" {
			t.Errorf("restored = %+v, %v", restored, err)
		}
	})

	t.Run("reports an update of no Product as not found", func(t *testing.T) {
		s := newSubject(t)

		_, err := s.Store.UpdateProduct(ctx, store.ChangeProductParams{ID: 99, Name: text("Thai tea")})

		if !errors.Is(err, store.ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("refuses what the rules refuse", func(t *testing.T) {
		s := newSubject(t)
		for name, bad := range map[string]store.InsertProductParams{
			"a blank name":           {Name: " ", Sku: "A", Unit: "pcs"},
			"a no-break space name":  {Name: " ", Sku: "A", Unit: "pcs"},
			"an ideographic name":    {Name: "　  ", Sku: "A", Unit: "pcs"},
			"a name over 120":        {Name: strings.Repeat("ก", 121), Sku: "A", Unit: "pcs"},
			"a blank unit":           {Name: "Tea", Sku: "A", Unit: ""},
			"a no-break space unit":  {Name: "Tea", Sku: "A", Unit: " "},
			"an ideographic unit":    {Name: "Tea", Sku: "A", Unit: "　"},
			"a unit over 20":         {Name: "Tea", Sku: "A", Unit: strings.Repeat("u", 21)},
			"a SKU with a space":     {Name: "Tea", Sku: "A B", Unit: "pcs"},
			"a SKU over 40":          {Name: "Tea", Sku: strings.Repeat("A", 41), Unit: "pcs"},
			"an empty SKU":           {Name: "Tea", Sku: "", Unit: "pcs"},
			"a negative price":       {Name: "Tea", Sku: "A", Unit: "pcs", Price: -1},
			"a price over 9,999,999": {Name: "Tea", Sku: "A", Unit: "pcs", Price: 1000000000},
			"a negative minimum":     {Name: "Tea", Sku: "A", Unit: "pcs", MinimumStock: -1},
		} {
			if _, err := s.Store.CreateProduct(ctx, bad); err == nil || errors.Is(err, store.ErrSKUTaken) {
				t.Errorf("%s: err = %v, want a refusal", name, err)
			}
		}
		for name, ok := range map[string]store.InsertProductParams{
			"a name of 120":         {Name: strings.Repeat("ก", 120), Sku: "A", Unit: "pcs"},
			"a unit of 20":          {Name: "Tea", Sku: "B", Unit: strings.Repeat("u", 20)},
			"a SKU of 40":           {Name: "Tea", Sku: strings.Repeat("A", 40), Unit: "pcs"},
			"a free product":        {Name: "Tea", Sku: "C-1", Unit: "pcs", Price: 0},
			"the highest price":     {Name: "Tea", Sku: "D", Unit: "pcs", Price: 999999999},
			"a minimum of 0":        {Name: "Tea", Sku: "E", Unit: "pcs", MinimumStock: 0},
			"a name in any script":  {Name: "ชาไทย", Sku: "F", Unit: "ชิ้น"},
			"a unit with a space":   {Name: "Tea", Sku: "G", Unit: "6 pack"},
			"a SKU of mixed things": {Name: "Tea", Sku: "aB-9-z", Unit: "pcs"},
		} {
			if _, err := s.Store.CreateProduct(ctx, ok); err != nil {
				t.Errorf("%s: err = %v, want it accepted", name, err)
			}
		}
	})

	// products R2, R4: blank in any script, on Update as well as Create.
	t.Run("refuses an update to a name or unit of only whitespace", func(t *testing.T) {
		s := newSubject(t)
		tea := s.AddProduct(t, store.Product{Name: "Tea", Sku: "A", Unit: "pcs", Status: "active"})
		for name, change := range map[string]store.ChangeProductParams{
			"a no-break space name": {ID: tea.ID, Name: text(" ")},
			"an ideographic name":   {ID: tea.ID, Name: text("　")},
			"a no-break space unit": {ID: tea.ID, Unit: text(" ")},
			"an ideographic unit":   {ID: tea.ID, Unit: text("　")},
		} {
			if _, err := s.Store.UpdateProduct(ctx, change); err == nil || errors.Is(err, store.ErrNotFound) {
				t.Errorf("%s: err = %v, want a refusal", name, err)
			}
		}
	})

	// products R9.
	t.Run("works out the stock state at each boundary", func(t *testing.T) {
		s := newSubject(t)
		for i, tc := range []struct {
			stock, minimum int32
			want           string
		}{
			{0, 0, "out-of-stock"},
			{0, 5, "out-of-stock"},
			{1, 0, "in-stock"},
			{1, 5, "low"},
			{5, 5, "low"},
			{6, 5, "in-stock"},
		} {
			p := s.AddProduct(t, store.Product{
				Name: "Tea", Sku: fmt.Sprintf("S%d", i), Unit: "pcs", Stock: tc.stock, MinimumStock: tc.minimum, Status: "active",
			})
			got, err := s.Store.GetProduct(ctx, p.ID)
			if err != nil || got.StockState != tc.want {
				t.Errorf("stock %d, minimum %d: state = %q, %v; want %q", tc.stock, tc.minimum, got.StockState, err, tc.want)
			}
			// The state follows a change of the Minimum stock too.
			if tc.minimum == 5 && tc.stock == 5 {
				changed, err := s.Store.UpdateProduct(ctx, store.ChangeProductParams{ID: p.ID, MinimumStock: pgtype.Int4{Int32: 4, Valid: true}})
				if err != nil || changed.StockState != "in-stock" {
					t.Errorf("minimum lowered to 4 under a stock of 5: state = %q, %v; want in-stock", changed.StockState, err)
				}
			}
		}
	})

	t.Run("lists Active Products by default, Archived ones on request, sorted by name", func(t *testing.T) {
		s := newSubject(t)
		for _, p := range []store.Product{
			{Name: "banana", Sku: "B"}, {Name: "Apple", Sku: "A"}, {Name: "Cherry", Sku: "C", Status: "archived"},
			{Name: "apple pie", Sku: "AP"}, {Name: "ส้ม", Sku: "T"}, {Name: "Apple", Sku: "A2"},
		} {
			p.Unit = "pcs"
			if p.Status == "" {
				p.Status = "active"
			}
			s.AddProduct(t, p)
		}

		active := listProducts(t, s, store.ListProductsParams{Status: "active"})
		archived := listProducts(t, s, store.ListProductsParams{Status: "archived"})

		if want := []string{"Apple", "Apple", "apple pie", "banana", "ส้ม"}; !slices.Equal(names(active), want) {
			t.Fatalf("active = %v, want %v", names(active), want)
		}
		if want := []string{"Cherry"}; !slices.Equal(names(archived), want) {
			t.Errorf("archived = %v, want %v", names(archived), want)
		}
		if active[0].Sku != "A" || active[1].Sku != "A2" {
			t.Errorf("equal names are not in the order they were added: %+v", active[:2])
		}
	})

	// products R14.
	t.Run("searches name and SKU without regard to letter case, and wildcards mean themselves", func(t *testing.T) {
		s := newSubject(t)
		for _, p := range []store.Product{
			{Name: "Thai Tea", Sku: "DRK-1"}, {Name: "Green tea", Sku: "DRK-2"}, {Name: "Rice", Sku: "tea-X"},
			{Name: "Soda", Sku: "DRK-3"}, {Name: "100% juice", Sku: "JUI"}, {Name: "Plain", Sku: "PLN"},
		} {
			p.Unit, p.Status = "pcs", "active"
			s.AddProduct(t, p)
		}

		byText := listProducts(t, s, store.ListProductsParams{Search: text("TEA")})
		bySKU := listProducts(t, s, store.ListProductsParams{Search: text("drk-")})
		percent := listProducts(t, s, store.ListProductsParams{Search: text("%")})
		underscore := listProducts(t, s, store.ListProductsParams{Search: text("_")})

		if want := []string{"Green tea", "Rice", "Thai Tea"}; !slices.Equal(names(byText), want) {
			t.Errorf("tea = %v, want %v", names(byText), want)
		}
		if want := []string{"Green tea", "Soda", "Thai Tea"}; !slices.Equal(names(bySKU), want) {
			t.Errorf("drk- = %v, want %v", names(bySKU), want)
		}
		if want := []string{"100% juice"}; !slices.Equal(names(percent), want) {
			t.Errorf("%% = %v, want %v", names(percent), want)
		}
		if len(underscore) != 0 {
			t.Errorf("_ = %v, want none", names(underscore))
		}
	})

	t.Run("filters by one or more stock states", func(t *testing.T) {
		s := newSubject(t)
		for _, p := range []store.Product{
			{Name: "A out", Stock: 0, MinimumStock: 2}, {Name: "B low", Stock: 2, MinimumStock: 2},
			{Name: "C in", Stock: 3, MinimumStock: 2}, {Name: "D low", Stock: 1, MinimumStock: 2},
		} {
			p.Sku, p.Unit, p.Status = p.Name[:1], "pcs", "active"
			s.AddProduct(t, p)
		}

		low := listProducts(t, s, store.ListProductsParams{StockStates: []string{"low"}})
		lowOrOut := listProducts(t, s, store.ListProductsParams{StockStates: []string{"low", "out-of-stock"}})
		count, err := s.Store.CountProducts(ctx, store.CountProductsParams{Status: "active", StockStates: []string{"low", "out-of-stock"}})

		if want := []string{"B low", "D low"}; !slices.Equal(names(low), want) {
			t.Errorf("low = %v, want %v", names(low), want)
		}
		if want := []string{"A out", "B low", "D low"}; !slices.Equal(names(lowOrOut), want) {
			t.Errorf("low or out = %v, want %v", names(lowOrOut), want)
		}
		if err != nil || count != 3 {
			t.Errorf("count = %d, %v; want 3", count, err)
		}
	})

	t.Run("pages the list and counts every match", func(t *testing.T) {
		s := newSubject(t)
		for i := range 5 {
			s.AddProduct(t, store.Product{Name: fmt.Sprintf("P%d", i), Sku: fmt.Sprintf("S%d", i), Unit: "pcs", Status: "active"})
		}

		second := listProducts(t, s, store.ListProductsParams{RowLimit: 2, RowOffset: 2})
		past := listProducts(t, s, store.ListProductsParams{RowLimit: 2, RowOffset: 10})
		count, err := s.Store.CountProducts(ctx, store.CountProductsParams{Status: "active"})

		if want := []string{"P2", "P3"}; !slices.Equal(names(second), want) {
			t.Errorf("second page = %v, want %v", names(second), want)
		}
		if len(past) != 0 {
			t.Errorf("past the end = %v, want none", names(past))
		}
		if err != nil || count != 5 {
			t.Errorf("count = %d, %v; want 5", count, err)
		}
	})
}
