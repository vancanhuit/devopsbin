
# TransferRequest


## Properties

Name | Type
------------ | -------------
`fromAccountId` | string
`toAccountId` | string
`amountCents` | number

## Example

```typescript
import type { TransferRequest } from ''

// TODO: Update the object below with actual values
const example = {
  "fromAccountId": 018f9d6b-cbbf-7b2d-9b5d-ab8dfbbd4bed,
  "toAccountId": 018f9d6b-cbbf-7b2d-9b5d-ab8dfbbd4bee,
  "amountCents": 2500,
} satisfies TransferRequest

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as TransferRequest
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


