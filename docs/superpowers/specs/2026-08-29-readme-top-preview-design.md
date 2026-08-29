# README Top Preview Design

## Goal

Make the README opening more persuasive by showing the working TUI immediately
after the product summary, before explaining the product model in detail.

## Structure

Apply the same order to both language variants:

1. Language switch, title, and badges.
2. Short product description and TUI/CLI positioning.
3. `Preview` / `界面预览`, using the existing GIF and caption unchanged.
4. `Why aikit` / `为什么选择 aikit`.
5. `Features` / `核心能力`.
6. The remaining README sections in their existing order.

Remove the complete `Project status` / `项目状态` section. Do not move its
alpha warning, release notes, scope list, or platform caveats elsewhere.
Supported-platform details remain available in the existing platform table,
and release history remains linked elsewhere in the repository.

## Documentation Contract

Update the documentation checker and its fixtures so they no longer require
the removed status headings. The checker must instead reject either removed
status heading if it reappears and verify that each README orders its localized
Preview, Why, and Features headings exactly in that sequence. Add failing
fixtures for a misplaced Preview and a restored status heading. Keep the
existing contract that both READMEs must reference the same non-empty GIF below
10 MiB.

## Boundaries

- Do not regenerate or edit the GIF.
- Do not change the introductory copy, Preview caption, feature list, install
  instructions, or later documentation.
- Keep English and Simplified Chinese structurally equivalent.

## Verification

- Confirm `Preview` appears before `Why aikit` in both READMEs.
- Confirm neither status heading nor its section body remains.
- Run the documentation checker self-tests and checker.
- Run `git diff --check` and inspect the final README opening order.
