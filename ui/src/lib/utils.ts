
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

const createNotificationSuccess = (notification: NotificationMessage, translator:any) => {
    return buildNotification({ ...notification, kind: "success", timeout: 2000 , title : translator("Success")}, translator);
}

const createNotificationError = (notification: NotificationMessage, translator:any) => {
    return buildNotification({ ...notification, kind: "error", timeout: 30000 }, translator);
}

const bytesToSize = (bytes: number) => {
    const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB']
    if (bytes === 0) return '0 B'
    const i = Math.floor(Math.log(bytes) / Math.log(1024))
    return Math.round(bytes / Math.pow(1024, i)) + ' ' + sizes[i]
}


export { buildNotification, createNotificationError, createNotificationSuccess, bytesToSize }