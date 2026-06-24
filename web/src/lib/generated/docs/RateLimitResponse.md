
# RateLimitResponse


## Properties

Name | Type
------------ | -------------
`limit` | number
`remaining` | number
`reset` | number

## Example

```typescript
import type { RateLimitResponse } from ''

// TODO: Update the object below with actual values
const example = {
  "limit": 5,
  "remaining": 4,
  "reset": 8,
} satisfies RateLimitResponse

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as RateLimitResponse
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


