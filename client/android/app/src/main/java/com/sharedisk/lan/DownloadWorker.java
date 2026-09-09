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

import org.json.JSONObject;

public final class DownloadWorker extends Worker {
    static final String TAG = "share-disk-download";

    public DownloadWorker(@NonNull Context context, @NonNull WorkerParameters params) {
        super(context, params);
    }

    static java.util.UUID enqueue(Context context, LanFile file, Uri destination) {
        Data input = new Data.Builder()
                .putString("file_json", toJSON(file).toString())
                .putString("destination", destination.toString())
                .build();
        Constraints constraints = new Constraints.Builder().setRequiredNetworkType(NetworkType.CONNECTED).build();
        OneTimeWorkRequest request = new OneTimeWorkRequest.Builder(DownloadWorker.class)
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
            setForegroundAsync(TransferNotifications.info(getApplicationContext(), getId(), "正在下载文件", "正在准备…"));
            LanFile file = new LanFile(new JSONObject(require("file_json")));
            Uri destination = Uri.parse(require("destination"));
            new ApiClient(getApplicationContext()).download(file, destination, message -> {
                setProgressAsync(new Data.Builder().putString("message", message).build());
                setForegroundAsync(TransferNotifications.info(getApplicationContext(), getId(), "正在下载 " + file.name, message));
            });
            return Result.success(new Data.Builder().putString("message", "下载完成并校验：" + file.name).build());
        } catch (Exception error) {
            String message = error.getMessage() == null ? error.getClass().getSimpleName() : error.getMessage();
			if (ApiClient.isRetryable(error) && getRunAttemptCount() < 5) {
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

    private static JSONObject toJSON(LanFile file) {
        JSONObject json = new JSONObject();
        try {
            return json.put("id", file.id).put("local_file_id", file.localFileId)
                    .put("object_id", file.objectId).put("name", file.name).put("mime", file.mime)
                    .put("size", file.size).put("sha256", file.sha256).put("status", file.status)
                    .put("replica_endpoint", file.replicaEndpoint).put("origin_endpoint", file.originEndpoint)
                    .put("content_local_file_id", file.contentLocalFileId).put("origin_device_name", file.originDeviceName)
                    .put("origin_device_id", file.originDeviceId);
        } catch (Exception impossible) {
            throw new IllegalStateException(impossible);
        }
    }
}
