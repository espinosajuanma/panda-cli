# Documentation

## Tasks

- Entity Name: `dev.tasks`

## Canonical task references

- Use `panda link <taskNumber|localId>` to generate a verified Markdown task link.
- The configured project view ID belongs in `views/`; never put the task's
  `entity.id` in this segment.
- The default task view ID is `680b8e41805d3740e97cfecb`. Configure project-specific
  view IDs with `panda config set project-view <viewId>`.
- Canonical form:
  `[#<taskNumber>](https://solutions.slingrs.io/prod/runtime/#views/<projectViewId>/readOnly/<verifiedTaskId>)`
- The `<verifiedTaskId>` is the task's `metadata.json` `id`, confirmed against
  Panda's last-synced remote baseline.
