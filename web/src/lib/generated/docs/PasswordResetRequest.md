
# PasswordResetRequest


## Properties

Name | Type
------------ | -------------
`token` | string
`newPassword` | string

## Example

```typescript
import type { PasswordResetRequest } from ''

// TODO: Update the object below with actual values
const example = {
  "token": 7Qy3w8Zk1pT0nB2cR4sV6xA9dE5fG1hJ3kL7mN0pQ,
  "newPassword": alicepass2,
} satisfies PasswordResetRequest

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as PasswordResetRequest
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


