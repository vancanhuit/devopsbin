
# VersionResponse


## Properties

Name | Type
------------ | -------------
`service` | string
`version` | string
`git_sha` | string
`build_time` | Date
`go_version` | string

## Example

```typescript
import type { VersionResponse } from ''

// TODO: Update the object below with actual values
const example = {
  "service": devopsbin-api,
  "version": 0.1.0,
  "git_sha": abc123,
  "build_time": 2026-06-06T10:00Z,
  "go_version": go1.24.4,
} satisfies VersionResponse

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as VersionResponse
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


