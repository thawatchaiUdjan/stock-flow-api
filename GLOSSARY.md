# stock-flow

The shared vocabulary of stock-flow, a small inventory and order system for one shop. All three repos use these terms.

## Language

### Users & access

**Shop**:
The one business a stock-flow system serves.
_Avoid_: Store, tenant, organization

**StockFlow**:
The app the Shop's Users work in, apart from the Storefront.
_Avoid_: Back office, admin panel

**User**:
A person who works in the Shop and can log in to StockFlow. Every User has exactly one Role.
_Avoid_: Account, member, employee

**Role**:
The level of a User, which sets what that User may reach.
_Avoid_: Permission, user type

**Active**:
The status of a User who may log in.
_Avoid_: Enabled

**Inactive**:
The status of a User who may no longer log in. The User still exists.
_Avoid_: Disabled, deactivated, deleted

**Owner**:
The Role of the Shop's owner: the highest Role, held by exactly one User, the first one in the system. Not the requirements owner, who decides this project's requirements.
_Avoid_: Super-admin

**Admin**:
The Role of a person who helps run the Shop, with everything the Owner has except managing Users.
_Avoid_: Manager

**Staff**:
The Role of a person who works in the Shop day to day.
_Avoid_: Employee, general user

**Setup link**:
A one-time link through which the first Owner sets their password at installation.
_Avoid_: Invite link, reset link, temporary password

**Login log**:
The permanent list of logins, failed logins, blocked logins and logouts of Users and Customers.
_Avoid_: Login history, sign-in log

**First-time code**:
A one-time code emailed to a new User to verify their email before they set their first password.
_Avoid_: Login OTP, temporary password

**Reset link**:
A one-time link emailed to a User who requests recovery of their password.
_Avoid_: Setup link, temporary password

### Products

**Product**:
Something the Shop sells and keeps in stock.
_Avoid_: Item, article

**SKU**:
The short code that identifies one Product.
_Avoid_: Code, product number, barcode

**Stock**:
How many units of a Product the Shop has and can still sell.
_Avoid_: Inventory, quantity on hand, balance

**Minimum stock**:
The level at or below which a Product needs restocking.
_Avoid_: Reorder point, threshold, safety stock

**Low**:
The state of a Product whose stock is above 0 and at or below its Minimum stock.
_Avoid_: Running out, warning

**Out of stock**:
The state of a Product whose stock is 0.
_Avoid_: Sold out, empty

**Archived**:
The status of a Product the Shop no longer sells. The Product still exists.
_Avoid_: Deleted, inactive, discontinued

### Stock movements

**Stock movement**:
One recorded change to a Product's stock, which is never edited.
_Avoid_: Transaction, stock log, ledger entry

**Receive**:
A Stock movement that adds units delivered to the Shop.
_Avoid_: Stock in, restock, purchase

**Adjustment**:
A Stock movement that corrects a Product's stock to what is really there, up or down.
_Avoid_: Correction, write-off, stock edit

### Orders

**Order**:
A sale of one or more Products to one customer.
_Avoid_: Sale, invoice, transaction

**Order line**:
One Product on an Order, with its quantity and price.
_Avoid_: Item, row

**Pending**:
The status of an Order that has taken its stock and is not finished yet.
_Avoid_: Open, new, draft

**Completed**:
The status of an Order the customer has paid for and received.
_Avoid_: Done, fulfilled, paid

**Cancelled**:
The status of an Order called off before it was Completed, whose stock went back.
_Avoid_: Voided, deleted

### Storefront

**Storefront**:
The Shop's public website, where anyone browses Products and Customers place Online orders.
_Avoid_: Online store, shop page, customer portal

**Customer**:
A person who buys from the Shop and can log in to the Storefront. A Customer is not a User and has no Role.
_Avoid_: Client, buyer, member, shopper, account

**Verification code**:
A one-time code emailed to a Customer to confirm their email before they set their password.
_Avoid_: OTP, First-time code, activation code

**Cart**:
The Products and quantities a Customer has picked on the Storefront and not yet ordered.
_Avoid_: Basket, bag, draft order

**Online order**:
An Order a Customer places on the Storefront.
_Avoid_: Web order, e-order, customer order

**In-shop order**:
An Order a User creates in StockFlow.
_Avoid_: Offline order, walk-in order, POS order

**Pickup**:
The way of receiving an Online order by collecting it at the Shop.
_Avoid_: Collection, self-pickup, click and collect

**Delivery**:
The way of receiving an Online order by having it sent to a Delivery address.
_Avoid_: Shipping, dispatch

**Delivery address**:
Where, and to whom, an Online order with Delivery is sent.
_Avoid_: Shipping address, destination

**Pay online**:
The way of paying for an Online order through the payment page before it is Completed.
_Avoid_: Prepaid, card payment, e-payment

**Pay on receipt**:
The way of paying for an Online order when the Customer receives it.
_Avoid_: Cash on delivery, COD, pay later

**Unpaid**:
The payment state of a Pay online Order whose payment has not arrived.
_Avoid_: Awaiting payment, outstanding

**Paid**:
The payment state of a Pay online Order whose payment has arrived.
_Avoid_: Settled, charged

**Refunded**:
The payment state of a Paid Order that was Cancelled and whose payment went back to the Customer.
_Avoid_: Reversed, returned

### Activity log

**Activity log**:
The permanent list of changes made to Products, Orders, Users and Customers: who made each, when, and what changed.
_Avoid_: Audit log, audit trail, history, change log

### Deleting

**Deleted**:
The status of a Product, User, Customer or Order the Shop has removed from sight and use. The record still exists, and what points at it still shows it.
_Avoid_: Removed, Trashed, Erased

### Dashboard & reports

**Sales**:
The totals of Completed Orders, counted on the day each was completed.
_Avoid_: Revenue, income, turnover
