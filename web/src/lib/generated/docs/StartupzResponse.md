
# StartupzResponse


## Properties

Name | Type
------------ | -------------
`status` | string
`checks` | [{ [key: string]: DependencyCheck; }](DependencyCheck.md)

## Example

```typescript
import type { StartupzResponse } from ''

// TODO: Update the object below with actual values
const example = {
  "status": started,
  "checks": {"config":{"status":"ok"},"migrations":{"status":"ok"}},
} satisfies StartupzResponse

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as StartupzResponse
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


