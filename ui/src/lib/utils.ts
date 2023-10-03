
type NotificationMessage = {
    kind?: "error" | "warning" | "info" | "success",
    title?: string,
    subtitle?: string,
    timeout?: number,
}
const buildNotification = (notification: NotificationMessage, translator:any) => {
    return {
        kind: notification.kind ?? "error",
        title: notification.title ?? translator('Error'),
        subtitle: notification.subtitle ?? translator("Unable to perform action"),
        timeout: notification.timeout ?? 30000,
    }
}

const buildNotificationSuccess = (notification: NotificationMessage, translator:any) => {
    return buildNotification({ ...notification, kind: "success", timeout: 30000 , title : translator("Success")}, translator);
}

const buildNotificationError = (notification: NotificationMessage, translator:any) => {
    return buildNotification({ ...notification, kind: "error", timeout: 2000 }, translator);
}



export { buildNotification, buildNotificationError, buildNotificationSuccess }