# stock-flow

The shared vocabulary of stock-flow, a small inventory and order system for one shop. All three repos use these terms.

## Language

### Users & access

**Shop**:
The one business a stock-flow system serves.
_Avoid_: Store, tenant, organization

**User**:
A person who can log in to the system. Every User has exactly one Role.
_Avoid_: Account, member

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
A one-time link through which a person sets their own password.
_Avoid_: Invite link, reset link, temporary password

**Login log**:
The permanent list of logins, failed logins and logouts.
_Avoid_: Audit log, login history

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

### Dashboard & reports

**Sales**:
The totals of Completed Orders, counted on the day each was completed.
_Avoid_: Revenue, income, turnover
