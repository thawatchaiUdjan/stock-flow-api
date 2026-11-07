package api_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

type salesDayBody struct {
	Date            string `json:"date"`
	Sales           int64  `json:"sales"`
	OrdersCompleted int64  `json:"ordersCompleted"`
}

type restockBody struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	SKU          string `json:"sku"`
	Unit         string `json:"unit"`
	Stock        int64  `json:"stock"`
	MinimumStock int64  `json:"minimumStock"`
	StockState   string `json:"stockState"`
}

type dashboardBody struct {
	Today              string          `json:"today"`
	Sales              int64           `json:"sales"`
	OrdersCompleted    int64           `json:"ordersCompleted"`
	PendingOrders      int64           `json:"pendingOrders"`
	LowProducts        int64           `json:"lowProducts"`
	OutOfStockProducts int64           `json:"outOfStockProducts"`
	SalesByDay         []salesDayBody  `json:"salesByDay"`
	Restock            []restockBody   `json:"restock"`
	NewestOrders       []orderListItem `json:"newestOrders"`
}

type reportProductBody struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	SKU          string `json:"sku"`
	QuantitySold int64  `json:"quantitySold"`
	Sales        int64  `json:"sales"`
}

type salesReportBody struct {
	From            string              `json:"from"`
	To              string              `json:"to"`
	Sales           int64               `json:"sales"`
	OrdersCompleted int64               `json:"ordersCompleted"`
	AverageSales    int64               `json:"averageSales"`
	SalesByDay      []salesDayBody      `json:"salesByDay"`
	TopProducts     []reportProductBody `json:"topProducts"`
}

// bangkokTime is an instant on a Bangkok date and time of 2026, the Shop's time
// (dashboard-and-reports R1).
func bangkokTime(month time.Month, day, hour, minute, second int) time.Time {
	return time.Date(2026, month, day, hour, minute, second, 0, time.FixedZone("Bangkok", 7*60*60))
}

// newHarnessAt is a harness whose clock starts at now, so that the sessions it logs in
// outlive the days a test looks back over.
func newHarnessAt(t *testing.T, now time.Time) *harness {
	t.Helper()
	h := newHarness(t)
	h.clock.Set(now)
	return h
}

// completeAt creates an Order of lines at when and completes it at when, then puts the
// clock back where it was.
func (h *harness) completeAt(when time.Time, cookie *http.Cookie, lines ...lineIn) orderBody {
	h.t.Helper()
	back := h.clock.Now()
	defer h.clock.Set(back)
	h.clock.Set(when)
	created := h.mustCreateOrder(cookie, lines...)
	rec := h.completeOrder(created.Number, cookie)
	if rec.Code != http.StatusOK {
		h.t.Fatalf("complete order: status = %d, want 200; body %s", rec.Code, rec.Body)
	}
	return decode[orderBody](h.t, rec)
}

func (h *harness) dashboard(cookies ...*http.Cookie) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.do(http.MethodGet, "/dashboard", nil, cookies...)
}

func (h *harness) mustDashboard(cookie *http.Cookie) dashboardBody {
	h.t.Helper()
	rec := h.dashboard(cookie)
	if rec.Code != http.StatusOK {
		h.t.Fatalf("dashboard: status = %d, want 200; body %s", rec.Code, rec.Body)
	}
	return decode[dashboardBody](h.t, rec)
}

// dashboardAt is the Dashboard as it is at now.
func (h *harness) dashboardAt(now time.Time, cookie *http.Cookie) dashboardBody {
	h.t.Helper()
	h.clock.Set(now)
	return h.mustDashboard(cookie)
}

func (h *harness) salesReport(query string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.do(http.MethodGet, "/reports/sales?"+query, nil, cookies...)
}

func (h *harness) mustSalesReport(query string, cookie *http.Cookie) salesReportBody {
	h.t.Helper()
	rec := h.salesReport(query, cookie)
	if rec.Code != http.StatusOK {
		h.t.Fatalf("sales report %q: status = %d, want 200; body %s", query, rec.Code, rec.Body)
	}
	return decode[salesReportBody](h.t, rec)
}

// sale seeds an Active Product with plenty of stock for the Orders a test completes.
func (h *harness) sale(name, sku string, price int64) store.Product {
	h.t.Helper()
	return h.seed(store.Product{Name: name, Sku: sku, Price: price, Stock: 1000})
}

func salesOn(days []salesDayBody, date string) salesDayBody {
	for _, d := range days {
		if d.Date == date {
			return d
		}
	}
	return salesDayBody{Date: "missing " + date}
}

// users-and-access R11, R23: Staff, and anyone without a live session, get neither the
// Dashboard nor the Sales report, whatever they ask for.
func TestStaffAndVisitorsAreRefusedTheDashboardAndTheSalesReport(t *testing.T) {
	h := newHarness(t)
	s := h.shop()

	for _, c := range []struct {
		name string
		get  func(...*http.Cookie) *httptest.ResponseRecorder
	}{
		{"dashboard", h.dashboard},
		{"sales report", func(cookies ...*http.Cookie) *httptest.ResponseRecorder { return h.salesReport("", cookies...) }},
		// A range that would be a 422 is a 403 first: who is refused never depends on the request.
		{"sales report of a bad range", func(cookies ...*http.Cookie) *httptest.ResponseRecorder {
			return h.salesReport("from=2026-11-05&to=2026-11-01", cookies...)
		}},
	} {
		assertProblem(t, c.get(s.staff), http.StatusForbidden, "")
		assertProblem(t, c.get(), http.StatusUnauthorized, "")
		for name, cookie := range map[string]*http.Cookie{"owner": s.owner, "admin": s.admin} {
			if rec := c.get(cookie); rec.Code == http.StatusForbidden || rec.Code == http.StatusUnauthorized {
				t.Errorf("%s as %s: status = %d, want it allowed", c.name, name, rec.Code)
			}
		}
	}
}

// dashboard-and-reports R3, R4, R5, R6: the Dashboard of a Shop with nothing in it is all
// zeros and empty lists, with a day for each of the 14.
func TestTheDashboardOfAnEmptyShopIsZeroWithNoGapsInTheChart(t *testing.T) {
	h := newHarness(t)
	s := h.shop() // 2026-11-06 09:00 UTC is 16:00 on the 6th in Bangkok

	rec := h.dashboard(s.owner)

	got := decode[dashboardBody](t, rec)
	if got.Today != "2026-11-06" || got.Sales != 0 || got.OrdersCompleted != 0 || got.PendingOrders != 0 ||
		got.LowProducts != 0 || got.OutOfStockProducts != 0 {
		t.Errorf("dashboard = %+v, want zeros for 2026-11-06", got)
	}
	if len(got.SalesByDay) != 14 || got.SalesByDay[0] != (salesDayBody{Date: "2026-10-24"}) || got.SalesByDay[13] != (salesDayBody{Date: "2026-11-06"}) {
		t.Errorf("sales by day = %+v, want 14 days of 0 from 2026-10-24 to 2026-11-06", got.SalesByDay)
	}
	for _, empty := range []string{`"restock":[]`, `"newestOrders":[]`} {
		if !strings.Contains(rec.Body.String(), empty) {
			t.Errorf("body %s, want %s", rec.Body, empty)
		}
	}
}

// dashboard-and-reports R2, R3, R4: today's Sales, the Orders completed today and the
// chart count Completed Orders only, each on the day it was completed. A Pending Order is
// counted as Pending, and a Cancelled one nowhere. The chart is the last 14 days, today
// included.
func TestTheDashboardCountsOnlyCompletedOrdersOnTheDayTheyWereCompleted(t *testing.T) {
	h := newHarness(t)
	s := h.shop() // now: 2026-11-06 16:00 in Bangkok
	tea := h.sale("Thai tea", "TEA-01", 4500)
	cup := h.sale("Cup", "CUP-01", 1000)
	h.completeAt(bangkokTime(11, 6, 9, 0, 0), s.staff, lineIn{tea.ID, 2})
	h.completeAt(bangkokTime(11, 6, 15, 0, 0), s.staff, lineIn{tea.ID, 1}, lineIn{cup.ID, 3})
	h.completeAt(bangkokTime(11, 5, 12, 0, 0), s.staff, lineIn{cup.ID, 1})
	h.completeAt(bangkokTime(10, 24, 0, 0, 0), s.staff, lineIn{cup.ID, 4})    // the chart's first day
	h.completeAt(bangkokTime(10, 23, 23, 59, 59), s.staff, lineIn{cup.ID, 8}) // the day before it
	h.mustCreateOrder(s.staff, lineIn{tea.ID, 7})                             // Pending
	cancelled := h.mustCreateOrder(s.staff, lineIn{tea.ID, 9})
	if rec := h.cancelOrder(cancelled.Number, s.owner); rec.Code != http.StatusOK {
		t.Fatalf("cancel: status = %d; body %s", rec.Code, rec.Body)
	}

	got := h.mustDashboard(s.admin)

	if want := int64(2*4500 + 4500 + 3*1000); got.Sales != want || got.OrdersCompleted != 2 || got.PendingOrders != 1 {
		t.Errorf("today: Sales %d, completed %d, pending %d; want %d, 2, 1", got.Sales, got.OrdersCompleted, got.PendingOrders, want)
	}
	if len(got.SalesByDay) != 14 {
		t.Fatalf("sales by day = %+v, want 14 days", got.SalesByDay)
	}
	for date, want := range map[string]salesDayBody{
		"2026-11-06": {Sales: got.Sales, OrdersCompleted: 2},
		"2026-11-05": {Sales: 1000, OrdersCompleted: 1},
		"2026-11-04": {},
		"2026-10-24": {Sales: 4000, OrdersCompleted: 1},
	} {
		want.Date = date
		if d := salesOn(got.SalesByDay, date); d != want {
			t.Errorf("%s = %+v, want %+v", date, d, want)
		}
	}
	if first := got.SalesByDay[0].Date; first != "2026-10-24" {
		t.Errorf("the chart starts on %s, want 2026-10-24", first)
	}
}

// dashboard-and-reports R1: an Order completed at 23:59:59 Bangkok time is Sales of that
// day, and one a second later is Sales of the next, which is also when "today" turns over.
func TestTheDashboardCutsDaysAtBangkokMidnight(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.sale("Thai tea", "TEA-01", 100)
	before := bangkokTime(11, 6, 23, 59, 59) // 2026-11-06 16:59:59 UTC
	after := bangkokTime(11, 7, 0, 0, 0)     // 2026-11-06 17:00:00 UTC
	h.completeAt(before, s.staff, lineIn{tea.ID, 1})
	h.completeAt(after, s.staff, lineIn{tea.ID, 2})

	atBefore := h.dashboardAt(before, s.owner)
	atAfter := h.dashboardAt(after, s.owner)

	if atBefore.Today != "2026-11-06" || atBefore.Sales != 100 || atBefore.OrdersCompleted != 1 {
		t.Errorf("at 23:59:59: today %s, Sales %d, completed %d; want 2026-11-06, 100, 1", atBefore.Today, atBefore.Sales, atBefore.OrdersCompleted)
	}
	if last := atBefore.SalesByDay[13]; last.Date != "2026-11-06" || last.Sales != 100 {
		t.Errorf("at 23:59:59 the chart ends with %+v, want 2026-11-06 with 100", last)
	}
	if atAfter.Today != "2026-11-07" || atAfter.Sales != 200 || atAfter.OrdersCompleted != 1 {
		t.Errorf("at 00:00:00: today %s, Sales %d, completed %d; want 2026-11-07, 200, 1", atAfter.Today, atAfter.Sales, atAfter.OrdersCompleted)
	}
	if d := salesOn(atAfter.SalesByDay, "2026-11-06"); d.Sales != 100 || d.OrdersCompleted != 1 {
		t.Errorf("at 00:00:00 yesterday = %+v, want 100 from 1 Order", d)
	}
	if atAfter.SalesByDay[0].Date != "2026-10-25" {
		t.Errorf("at 00:00:00 the chart starts on %s, want 2026-10-25", atAfter.SalesByDay[0].Date)
	}
}

// dashboard-and-reports R3: only Active Products are counted as Low or Out of stock.
func TestTheDashboardCountsTheActiveProductsThatAreLowAndOutOfStock(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	for i, p := range []store.Product{
		{Name: "Plenty", Stock: 100, MinimumStock: 10},
		{Name: "At minimum", Stock: 10, MinimumStock: 10},
		{Name: "Few", Stock: 1, MinimumStock: 10},
		{Name: "Gone", Stock: 0, MinimumStock: 10},
		{Name: "Gone too", Stock: 0},
		{Name: "Archived gone", Stock: 0, Status: "archived"},
		{Name: "Archived few", Stock: 1, MinimumStock: 10, Status: "archived"},
	} {
		p.Sku = fmt.Sprintf("P-%d", i)
		h.seed(p)
	}

	got := h.mustDashboard(s.owner)

	if got.LowProducts != 2 || got.OutOfStockProducts != 2 {
		t.Errorf("Low %d, Out of stock %d; want 2 and 2", got.LowProducts, got.OutOfStockProducts)
	}
}

// dashboard-and-reports R5: up to 10 Active Products, Out of stock first and then the lowest
// stock, each with its stock, Minimum stock and state.
func TestTheDashboardListsTenProductsToRestockWorstFirst(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	h.seed(store.Product{Name: "Plenty", Sku: "P-0", Stock: 100, MinimumStock: 10})
	h.seed(store.Product{Name: "Archived", Sku: "P-1", Stock: 0, Status: "archived"})
	few := h.seed(store.Product{Name: "Few", Sku: "P-2", Stock: 3, MinimumStock: 10, Unit: "box"})
	gone := h.seed(store.Product{Name: "Gone", Sku: "P-3", Stock: 0, MinimumStock: 10})
	h.seed(store.Product{Name: "Z-low", Sku: "P-4", Stock: 4, MinimumStock: 10})

	got := h.mustDashboard(s.owner).Restock

	if len(got) != 3 || got[0].ID != gone.ID || got[1].ID != few.ID || got[2].Name != "Z-low" {
		t.Fatalf("restock = %+v, want Gone, Few, Z-low", got)
	}
	if want := (restockBody{ID: few.ID, Name: "Few", SKU: "P-2", Unit: "box", Stock: 3, MinimumStock: 10, StockState: "low"}); got[1] != want {
		t.Errorf("Few = %+v, want %+v", got[1], want)
	}
	if got[0].StockState != "out-of-stock" || got[0].Stock != 0 {
		t.Errorf("Gone = %+v, want out of stock", got[0])
	}

	for i := range 12 {
		h.seed(store.Product{Name: fmt.Sprintf("Out %02d", i), Sku: fmt.Sprintf("O-%d", i), Stock: 0})
	}
	if many := h.mustDashboard(s.owner).Restock; len(many) != 10 {
		t.Errorf("restock has %d Products, want at most 10", len(many))
	}
}

// dashboard-and-reports R6: the 5 newest Orders, newest first, as List Orders gives them.
func TestTheDashboardListsTheFiveNewestOrders(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.sale("Thai tea", "TEA-01", 100)
	for range 7 {
		h.mustCreateOrder(s.staff, lineIn{tea.ID, 1})
	}

	got := h.mustDashboard(s.admin).NewestOrders

	var numbers []int64
	for _, o := range got {
		numbers = append(numbers, o.Number)
	}
	if want := []int64{7, 6, 5, 4, 3}; !slices.Equal(numbers, want) {
		t.Errorf("newest = %v, want %v", numbers, want)
	}
	if got[0].Total != 100 || got[0].Status != "pending" || got[0].LineCount != 1 {
		t.Errorf("newest = %+v, want an Order as List Orders gives it", got[0])
	}
}

// dashboard-and-reports R7: this month so far when from and to are left out; both ends are
// included; a day with no Sales is 0.
func TestTheSalesReportDefaultsToThisMonthSoFar(t *testing.T) {
	h := newHarnessAt(t, bangkokTime(11, 20, 10, 0, 0))
	s := h.shop()
	tea := h.sale("Thai tea", "TEA-01", 500)
	h.completeAt(bangkokTime(10, 31, 23, 59, 59), s.staff, lineIn{tea.ID, 9}) // last month
	h.completeAt(bangkokTime(11, 1, 0, 0, 0), s.staff, lineIn{tea.ID, 1})
	h.completeAt(bangkokTime(11, 20, 9, 0, 0), s.staff, lineIn{tea.ID, 2})
	h.completeAt(bangkokTime(11, 21, 0, 0, 0), s.staff, lineIn{tea.ID, 4}) // tomorrow
	h.clock.Set(bangkokTime(11, 20, 10, 0, 0))

	got := h.mustSalesReport("", s.owner)

	if got.From != "2026-11-01" || got.To != "2026-11-20" || got.Sales != 1500 || got.OrdersCompleted != 2 || got.AverageSales != 750 {
		t.Errorf("report = %+v, want 2026-11-01 to 2026-11-20 with 1500 from 2 Orders, 750 each", got)
	}
	if len(got.SalesByDay) != 20 || got.SalesByDay[0] != (salesDayBody{"2026-11-01", 500, 1}) ||
		got.SalesByDay[19] != (salesDayBody{"2026-11-20", 1000, 1}) || got.SalesByDay[5] != (salesDayBody{Date: "2026-11-06"}) {
		t.Errorf("sales by day = %+v, want a day for each of the 20 with 0 for the empty ones", got.SalesByDay)
	}
}

// dashboard-and-reports R1, R7: both ends of the range are whole Bangkok days, cut at
// Bangkok midnight.
func TestTheSalesReportIncludesBothEndsOfTheRangeAndCutsAtBangkokMidnight(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.sale("Thai tea", "TEA-01", 100)
	h.completeAt(bangkokTime(11, 2, 23, 59, 59), s.staff, lineIn{tea.ID, 1}) // before the range
	h.completeAt(bangkokTime(11, 3, 0, 0, 0), s.staff, lineIn{tea.ID, 2})    // its first instant
	h.completeAt(bangkokTime(11, 4, 23, 59, 59), s.staff, lineIn{tea.ID, 3}) // its last
	h.completeAt(bangkokTime(11, 5, 0, 0, 0), s.staff, lineIn{tea.ID, 4})    // after it

	got := h.mustSalesReport("from=2026-11-03&to=2026-11-04", s.admin)

	if got.Sales != 500 || got.OrdersCompleted != 2 || len(got.SalesByDay) != 2 ||
		got.SalesByDay[0] != (salesDayBody{"2026-11-03", 200, 1}) || got.SalesByDay[1] != (salesDayBody{"2026-11-04", 300, 1}) {
		t.Errorf("report = %+v, want 500 from 2 Orders over 2026-11-03 and 2026-11-04", got)
	}
	if one := h.mustSalesReport("from=2026-11-04&to=2026-11-04", s.admin); one.Sales != 300 || len(one.SalesByDay) != 1 {
		t.Errorf("one day = %+v, want 300 over one day", one)
	}
}

// dashboard-and-reports R2: Pending and Cancelled Orders are not Sales, in the totals, the
// days or the Products.
func TestTheSalesReportCountsOnlyCompletedOrders(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.sale("Thai tea", "TEA-01", 100)
	h.completeAt(bangkokTime(11, 6, 9, 0, 0), s.staff, lineIn{tea.ID, 1})
	h.mustCreateOrder(s.staff, lineIn{tea.ID, 10})
	cancelled := h.mustCreateOrder(s.staff, lineIn{tea.ID, 20})
	if rec := h.cancelOrder(cancelled.Number, s.owner); rec.Code != http.StatusOK {
		t.Fatalf("cancel: status = %d; body %s", rec.Code, rec.Body)
	}

	got := h.mustSalesReport("from=2026-11-01&to=2026-11-30", s.owner)

	if got.Sales != 100 || got.OrdersCompleted != 1 || len(got.TopProducts) != 1 || got.TopProducts[0].QuantitySold != 1 || got.TopProducts[0].Sales != 100 {
		t.Errorf("report = %+v, want only the Completed Order", got)
	}
}

// The average Sales per Order is the total over the Orders, rounded half up to the satang,
// and 0 when there are none.
func TestTheAverageSalesPerOrderRoundsHalfUpToTheSatang(t *testing.T) {
	for _, c := range []struct {
		name   string
		totals []int64
		want   int64
	}{
		{"no Orders", nil, 0},
		{"exact", []int64{100, 300}, 200},
		{"exactly half goes up", []int64{100, 101}, 101},      // 100.5
		{"under half goes down", []int64{100, 100, 101}, 100}, // 100.33
		{"over half goes up", []int64{100, 101, 101}, 101},    // 100.67
		{"one Order", []int64{7}, 7},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			s := h.shop()
			for i, total := range c.totals {
				p := h.sale(fmt.Sprintf("Product %d", i), fmt.Sprintf("P-%d", i), total)
				h.completeAt(bangkokTime(11, 6, 9, 0, 0), s.staff, lineIn{p.ID, 1})
			}

			got := h.mustSalesReport("from=2026-11-06&to=2026-11-06", s.owner)

			if got.AverageSales != c.want {
				t.Errorf("average = %d, want %d (report %+v)", got.AverageSales, c.want, got)
			}
		})
	}
}

// dashboard-and-reports R7: up to 366 days; from after to is refused.
func TestTheSalesReportRefusesARangeOverTheLimitOrBackwards(t *testing.T) {
	h := newHarness(t)
	s := h.shop()

	if got := h.mustSalesReport("from=2025-11-07&to=2026-11-07", s.owner); len(got.SalesByDay) != 366 || got.SalesByDay[0].Date != "2025-11-07" {
		t.Errorf("366 days: got %d days from %s", len(got.SalesByDay), got.From)
	}
	if got := h.mustSalesReport("from=2024-02-28&to=2025-02-27", s.owner); len(got.SalesByDay) != 366 {
		t.Errorf("366 days over a leap day: got %d days", len(got.SalesByDay))
	}
	for name, query := range map[string]string{
		"367 days":             "from=2025-11-06&to=2026-11-07",
		"367 days over a leap": "from=2024-02-28&to=2025-02-28",
		"a century":            "from=1926-11-07&to=2026-11-07",
		"from after to":        "from=2026-11-08&to=2026-11-07",
		"from after today":     "from=2026-11-07",
		"not a date":           "from=yesterday",
		"an impossible date":   "from=2026-02-30&to=2026-03-01",
	} {
		t.Run(name, func(t *testing.T) {
			assertProblem(t, h.salesReport(query, s.owner), http.StatusUnprocessableEntity, "")
		})
	}
}

// dashboard-and-reports R8: up to 20 Products with the highest Sales in the range, each with
// the quantity sold and its Sales.
func TestTheSalesReportListsTheTwentyProductsWithTheHighestSales(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	var lines []lineIn
	for i := range 22 {
		p := h.sale(fmt.Sprintf("Product %02d", i), fmt.Sprintf("P-%02d", i), int64(100*(i+1)))
		lines = append(lines, lineIn{p.ID, 2})
	}
	h.completeAt(bangkokTime(11, 6, 9, 0, 0), s.staff, lines...)

	got := h.mustSalesReport("from=2026-11-06&to=2026-11-06", s.owner).TopProducts

	if len(got) != 20 {
		t.Fatalf("top products = %d, want 20", len(got))
	}
	if want := (reportProductBody{ID: got[0].ID, Name: "Product 21", SKU: "P-21", QuantitySold: 2, Sales: 2 * 2200}); got[0] != want {
		t.Errorf("first = %+v, want %+v", got[0], want)
	}
	if got[19].Name != "Product 02" || got[19].Sales != 2*300 {
		t.Errorf("last = %+v, want Product 02 with 600", got[19])
	}
	for i := 1; i < len(got); i++ {
		if got[i].Sales > got[i-1].Sales {
			t.Errorf("product %d sold for %d, more than the one before (%d)", i, got[i].Sales, got[i-1].Sales)
		}
	}
}

// The prices of an Order never change (orders R3), so neither do its Sales when the Product
// is repriced or Archived later.
func TestTheSalesReportKeepsTheSalesOfAProductRepricedOrArchivedLater(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.sale("Thai tea", "TEA-01", 4500)
	h.completeAt(bangkokTime(11, 6, 9, 0, 0), s.staff, lineIn{tea.ID, 2})
	if rec := h.updateProduct(tea.ID, map[string]any{"price": 9900, "status": "archived"}, s.owner); rec.Code != http.StatusOK {
		t.Fatalf("update: status = %d; body %s", rec.Code, rec.Body)
	}

	got := h.mustSalesReport("from=2026-11-06&to=2026-11-06", s.owner)

	if got.Sales != 9000 || len(got.TopProducts) != 1 || got.TopProducts[0].Sales != 9000 || got.TopProducts[0].QuantitySold != 2 {
		t.Errorf("report = %+v, want 9000 from 2 units of the Archived Product", got)
	}
}

// A Shop that sold nothing in the range has a report of zeros and empty lists.
func TestTheSalesReportOfADayWithNoSalesIsZero(t *testing.T) {
	h := newHarness(t)
	s := h.shop()

	rec := h.salesReport("from=2026-11-01&to=2026-11-02", s.owner)

	got := decode[salesReportBody](t, rec)
	if got.Sales != 0 || got.OrdersCompleted != 0 || got.AverageSales != 0 ||
		!slices.Equal(got.SalesByDay, []salesDayBody{{Date: "2026-11-01"}, {Date: "2026-11-02"}}) {
		t.Errorf("report = %+v, want zeros over two days", got)
	}
	if !strings.Contains(rec.Body.String(), `"topProducts":[]`) {
		t.Errorf("body %s, want an empty list of Products", rec.Body)
	}
}
