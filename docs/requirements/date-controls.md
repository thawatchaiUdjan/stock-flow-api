# Date controls

Status: Confirmed (2026-11-09, agent under the requirements owner's task-scoped Autopilot delegation, ADR 0008)

## Purpose

Every User can recognise, choose and remove optional dates consistently throughout StockFlow. Date controls distinguish an unbounded history filter from Reports' default range, while keeping the Shop's calendar days intact.

## Rules

- **R1** Every date picker follows these rules, including both From/To controls on Stock movements and Reports; this does not add date filters to pages that have none.
- **R2** From and To remain independent controls with persistent visible, accessible labels; an unset From shows "Start date" and an unset To shows "End date" inside the control. Other date pickers use a purpose-specific empty prompt and label.
- **R3** A selected date appears inside its control with day, abbreviated month and full year (for example, "5 Nov 2026"); no redundant selected-date line appears beneath it. Date pickers have no Today or Clear action, nor a replacement clear icon or button.
- **R4** A User unsets an optional date by activating its selected day again in the calendar, with pointer, touch or keyboard. The calendar explains "Select the selected day again to remove it" when a value is selected. Unsetting affects only that bound; required dates, if any, cannot be unset this way.
- **R5** Opening a calendar focuses the selected day, or today in Bangkok if unset; opening, navigating or dismissing it alone never changes the value. Choosing or unsetting a day closes it and returns focus to its control; Escape closes it without changing the value. Month/year navigation, day selection, selected state and the removal instruction are accessible with a keyboard and screen reader.
- **R6** Stock movements leaves an unset bound unbounded and shows the shared hint "Leave dates empty for all history". Either bound can be used alone; other filters remain in effect when a date is unset.
- **R7** Reports keeps the existing defaults: an unset To means today in Bangkok, and an unset From means the first day of the effective To month. Both unset therefore means this month so far, never all history. A shared explanation beside the controls states these defaults and shows the effective From/To dates even during loading or errors; successfully returned figures use the range returned with those figures.
- **R8** Effective defaults never appear as explicit selections or get written to the address just by opening or viewing the page. Explicit date selections survive paging, reloads and shared links; setting or unsetting a bound returns to the first/newest page and preserves unrelated filters. Reports' existing "This month so far" action remains a range reset, not a per-date Clear action.
- **R9** Dates remain Bangkok calendar days regardless of the device time zone (Dashboard & reports R1). Reversed ranges and Reports ranges over 366 inclusive days keep their existing refusal behavior; the control does not silently swap, clamp or fill bounds to make them valid.
- **R10** The full picker, its prompts, dates and guidance remain readable and operable on narrow screens without horizontal page overflow; keyboard focus is visible and does not become trapped after dismissal.

## Out of scope

- Changing API contracts, stored data, Role access, Sales calculations, range limits or adding new date filters.
- Choosing a component library or internal implementation; the web repo owns these and updates its design direction to realise these rules.
- Removing existing whole-page filter-reset actions outside the date picker.

## Open questions

None. The requirements owner selected Autopilot on 2026-11-09 for adjusting all date pickers according to context PR #32. The agent adopted its proposal and decided optional-date deselection and Reports' shared effective-range guidance under ADR 0008. The [research note](../research/2026-11-09-date-picker-empty-state.md) is historical evidence; these rules adopt and settle its proposal. No existing confirmed business rule or accepted ADR is changed, and no new domain term is introduced.
