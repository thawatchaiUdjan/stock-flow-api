# Date-picker empty states without Today/Clear buttons

Checked: 2026-11-09. Work mode: Autopilot, selected by the requirements owner for research and documentation under ADR 0008. This note provides evidence and recommendations; it does not adopt a web design change or authorise implementation.

## Question

How can every From/To date control show useful text before a date is selected, without the Today and Clear buttons shown beneath the current controls?

## Current project context

The web repo's [design direction, Layout](https://github.com/thawatchaiUdjan/stock-flow-web/blob/main/docs/design.md#layout) currently prescribes native date inputs, a separate full-year day display and Today/Clear buttons. It distinguishes unbounded history filters from Reports' effective defaults. Changing this presentation requires updating that source in the web repo, rather than copying its design rules here. [Dashboard & reports R1 and R7](../requirements/dashboard-and-reports.md) establish Bangkok calendar days and the Reports default of this month so far.

## Findings

- Native `input type="date"` does not support the `placeholder` attribute. Setting a custom placeholder string on that input cannot reliably replace its date segments. Its displayed date format follows browser locale, and the picker appearance varies with browser and operating system. Sources: [WHATWG Date state](https://html.spec.whatwg.org/multipage/input.html#date-state-(type=date)), [MDN date input](https://developer.mozilla.org/en-US/docs/Web/HTML/Reference/Elements/input/date).
- A custom trigger can display text when empty and a formatted date when selected. The official shadcn example composes Button, Popover and Calendar and switches between a selected date and the text “Pick a date”. Its example does not add Today/Clear actions. This demonstrates an approach, not a mandate to install the newest library or replace existing primitives. Source: [shadcn Radix Date Picker](https://ui.shadcn.com/docs/components/radix/date-picker).
- Empty-state text must not replace a persistent accessible label. Placeholders disappear after entry and are not an adequate substitute for labels. Source: [MDN placeholder accessibility](https://developer.mozilla.org/en-US/docs/Web/HTML/Reference/Attributes/placeholder#accessibility_concerns).
- A calendar can open with focus on today when the field is empty without selecting a value. This offers a useful starting month without a separate Today action. The WAI example also describes keyboard navigation and focus handling; it warns that production use requires assistive-technology testing. Source: [WAI APG Date Picker Dialog](https://www.w3.org/WAI/ARIA/apg/patterns/dialog-modal/examples/datepicker-dialog/).
- The native date input specification does not expose a switch for hiding individual Today/Clear controls inside the browser-owned calendar. Removing application-rendered buttons beneath the field is separate from controlling the browser popup. Sources: [WHATWG Date state](https://html.spec.whatwg.org/multipage/input.html#date-state-(type=date)), [MDN date input](https://developer.mozilla.org/en-US/docs/Web/HTML/Reference/Elements/input/date).

## Recommended presentation (proposal)

Keep the two independent From/To controls and their labels. Use a custom calendar trigger consistent with the existing web design. Remove the Today/Clear button row and redundant selected-date text below each control; show the selected full-year date in the control itself.

| State | From | To |
| --- | --- | --- |
| No explicit selection | Start date | End date |
| Selected example | 5 Nov 2026 | 9 Nov 2026 |

For unbounded history filters, one shared hint such as “Leave dates empty for all history” can explain the empty values. This wording and the table are design recommendations, not sourced requirements. Reports must communicate the effective default range rather than implying that an unset bound means all history; show the effective dates or explain those defaults beside the range without silently changing URL state.

Removing Clear must still leave a usable route back to an unset optional bound. Evaluate a calendar that permits deselecting the selected day, or an editable field that permits deleting its contents; do not assume the chosen component supports either. Do not silently replace Clear with another visible clear button. An editable option must define formatting and invalid-input behavior. Verify keyboard operation, focus return, screen-reader labels and narrow-screen layout across every use of the shared control.

## Next step

In stock-flow-web, inspect all usages and the installed component versions, update its design direction, and implement the agreed presentation there. This research task changes no requirement, API contract or implementing code. No new domain vocabulary is introduced. A feature-log entry is not added because this task is research only, rather than a new feature or requirement change begun with `/grill-with-docs`.
