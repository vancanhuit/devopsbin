
# EchoResponse


## Properties

Name | Type
------------ | -------------
`method` | string
`path` | string
`query` | { [key: string]: Array&lt;string&gt;; }
`headers` | { [key: string]: Array&lt;string&gt;; }
`origin` | string

## Example

```typescript
import type { EchoResponse } from ''

// TODO: Update the object below with actual values
const example = {
  "method": GET,
  "path": /api/v1/echo,
  "query": {"Accept":["application/json"],"User-Agent":["curl/8.7.1"]},
  "headers": {"Accept":["application/json"],"User-Agent":["curl/8.7.1"]},
  "origin": 203.0.113.42,
} satisfies EchoResponse

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as EchoResponse
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


