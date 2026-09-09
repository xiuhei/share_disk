package com.sharedisk.lan;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.content.Context;
import android.content.Intent;
import android.content.pm.ServiceInfo;

import androidx.work.ForegroundInfo;
import androidx.work.WorkManager;

import java.util.UUID;

/** Builds the foreground notification used by long-running file transfers. */
final class TransferNotifications {
    private static final String CHANNEL_ID = "share_disk_transfers";

    private TransferNotifications() {}

    static ForegroundInfo info(Context context, UUID workId, String title, String message) {
        NotificationManager manager = (NotificationManager) context.getSystemService(Context.NOTIFICATION_SERVICE);
        NotificationChannel channel = new NotificationChannel(
                CHANNEL_ID, "文件传输", NotificationManager.IMPORTANCE_LOW);
        channel.setDescription("显示 Share Disk 上传和下载进度");
        manager.createNotificationChannel(channel);

        Intent launchIntent = new Intent(context, MainActivity.class)
                .putExtra("open_section", "transfers")
                .addFlags(Intent.FLAG_ACTIVITY_CLEAR_TOP | Intent.FLAG_ACTIVITY_SINGLE_TOP);
        PendingIntent launch = PendingIntent.getActivity(context, notificationId(workId), launchIntent,
                PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE);
        Notification.Builder builder = new Notification.Builder(context, CHANNEL_ID);
        Notification notification = builder
                .setSmallIcon(R.drawable.ic_transfer)
                .setContentTitle(title)
                .setContentText(message)
                .setCategory(Notification.CATEGORY_PROGRESS)
                .setOnlyAlertOnce(true)
                .setOngoing(true)
                .setContentIntent(launch)
                .setProgress(0, 0, true)
                .addAction(android.R.drawable.ic_menu_close_clear_cancel, "取消",
                        WorkManager.getInstance(context).createCancelPendingIntent(workId))
                .build();
        return new ForegroundInfo(notificationId(workId), notification,
                ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC);
    }

    private static int notificationId(UUID workId) {
        return 10_000 + Math.abs(workId.hashCode() % 20_000);
    }
}
