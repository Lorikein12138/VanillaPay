import { http, HttpResponse } from 'msw'

export const emptyNotifications = http.get('*/api/v1/merchant/notifications', () => HttpResponse.json({ data: { items: [], next_cursor: null, total: 0, unread_count: 0, popup_announcements: [] }, error: null, request_id: crypto.randomUUID() }))
