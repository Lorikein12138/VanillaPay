// The signed URL belongs to the server. Preserve its exact bytes, including query.
export function returnToMerchant(url: string) { window.location.assign(url) }
