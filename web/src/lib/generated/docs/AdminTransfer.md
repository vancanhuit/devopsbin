
# AdminTransfer


## Properties

Name | Type
------------ | -------------
`id` | string
`fromAccountId` | string
`fromAccountName` | string
`toAccountId` | string
`toAccountName` | string
`amountCents` | number
`createdAt` | Date

## Example

```typescript
import type { AdminTransfer } from ''

// TODO: Update the object below with actual values
const example = {
  "id": 018f9d6b-cbbf-7b2d-9b5d-ab8dfbbd4bed,
  "fromAccountId": 018f9d6b-cbbf-7b2d-9b5d-ab8dfbbd4bed,
  "fromAccountName": Checking,
  "toAccountId": 018f9d6b-cbbf-7b2d-9b5d-ab8dfbbd4bee,
  "toAccountName": Savings,
  "amountCents": 2500,
  "createdAt": 2025-01-15T09:30Z,
} satisfies AdminTransfer

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as AdminTransfer
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


