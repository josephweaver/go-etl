Goal:
Make the LandCore field-year-crop synthetic workflow admission failure observable and reproducible, then fix the concrete admission/compile defect.

Allowed production files:
- cmd/controller/main.go
- cmd/controller/*_test.go
- internal/workflow/* only if the failing error is inside workflow compile
- internal/reposource/* only if the failing error is inside source admission/cache prep
- internal/persistence/* only if the failing error is inside workflow-run creation

Do not touch:
- worker spin-up framework
- worker runtime behavior
- LandCore Python scripts
- geospatial worker plugin behavior
- field-year-crop workflow semantics unless the test proves the workflow is invalid

Required implementation:
1. Add a controller admission test that uses a minimal local source tree matching the field-year-crop smoke shape.
2. Submit the workflow through the same `POST /workflow` handler path.
3. Assert that failure responses include the underlying admission phase and error message.
4. Fix the actual admission/compile bug once visible.
5. Assert that the synthetic workflow creates:
   - workflow run
   - stage plan
   - initial queued work items