# Revision Notes

Date: 2026-07-06

This package revises the Data Assets and Materialized Outputs Strategic Concept with the following changes:

- keeps the worker-phase `get_data` / `put_data` design rather than introducing primary data-movement work item types;
- adds explicit provider families: `http`, `local_file`, `registered_location`, and `gdrive_rclone`;
- adds `gdrive_rclone` as a narrow adapter over a configured rclone executable, with fake-rclone tests and no real Google Drive dependency in default tests;
- adds cache policy semantics, including `cache.immutable`, cache keys, and cache-manifest reuse rules;
- adds integrity checks for expected SHA-256 and expected byte count;
- adds archive-selection declarations and a new worker archive extraction slice;
- makes ZIP selected-member extraction a phase-1 target;
- reserves `seven_zip` / `7z` for Yan/Roy-style `.7z` archives through a configured external executable, without requiring real 7z in default tests;
- updates the CDL/Yan/Roy fixture pipeline to produce field/CDL composition first, with dominant-crop assignment as a declared downstream policy input to RCI;
- renumbers later slices so archive extraction and gdrive-rclone provider support occur before Python argument binding, publication, fake-HPCC smoke, and the CDL/Yan/Roy fixture pipeline.

The key design conclusion is still that phase 1 does not need primary `get_data` and `put_data` work item types. Input materialization, archive extraction, and output publication should be worker runtime phases around plugin execution. Standalone data-movement work item types can be added later if workflows need them as explicit stage-visible operations.
