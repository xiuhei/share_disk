package com.sharedisk.lan;

import android.content.Context;
import android.net.Uri;

import androidx.annotation.NonNull;
import androidx.work.Constraints;
import androidx.work.Data;
import androidx.work.NetworkType;
import androidx.work.OneTimeWorkRequest;
import androidx.work.WorkManager;
import androidx.work.Worker;
import androidx.work.WorkerParameters;

/** Durable SAF-to-tus upload that survives Activity and process recreation. */
public final class UploadWorker extends Worker {
    static final String TAG = "share-disk-upload";

    public UploadWorker(@NonNull Context context, @NonNull WorkerParameters params) {
        super(context, params);
    }

    static java.util.UUID enqueue(Context context, Uri source) {
        Data input = new Data.Builder().putString("source", source.toString()).build();
        Constraints constraints = new Constraints.Builder().setRequiredNetworkType(NetworkType.CONNECTED).build();
        OneTimeWorkRequest request = new OneTimeWorkRequest.Builder(UploadWorker.class)
                .setInputData(input)
                .setConstraints(constraints)
                .addTag(TAG)
                .build();
        WorkManager.getInstance(context).enqueue(request);
        return request.getId();
    }

    @NonNull
    @Override
    public Result doWork() {
        try {
            setForegroundAsync(TransferNotifications.info(getApplicationContext(), getId(), "正在上传文件", "正在准备…"));
            Uri source = Uri.parse(require("source"));
            LanFile file = new ApiClient(getApplicationContext()).upload(source, message -> {
                setProgressAsync(new Data.Builder().putString("message", message).build());
                setForegroundAsync(TransferNotifications.info(getApplicationContext(), getId(), "正在上传文件", message));
            });
            String message = file.status.equals("local_ready_pending_register")
                    ? "本地上传完成并校验：" + file.name + "；控制服务恢复后将自动登记"
                    : "上传完成并校验：" + file.name;
            return Result.success(new Data.Builder().putString("message", message).build());
        } catch (Exception error) {
            String message = error.getMessage() == null ? error.getClass().getSimpleName() : error.getMessage();
            if (ApiClient.isRetryable(error) && getRunAttemptCount() < 20) {
                setProgressAsync(new Data.Builder().putString("message", "等待重试：" + message).build());
                return Result.retry();
            }
            return Result.failure(new Data.Builder().putString("message", message).build());
        }
    }

    private String require(String key) {
        String value = getInputData().getString(key);
        if (value == null || value.isEmpty()) throw new IllegalArgumentException("missing " + key);
        return value;
    }
}
