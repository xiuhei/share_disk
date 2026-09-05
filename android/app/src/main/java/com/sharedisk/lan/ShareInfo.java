package com.sharedisk.lan;

import org.json.JSONObject;

final class ShareInfo {
    final String id;
    final String fileName;
    final String status;
    final String expiresAt;
    final int maxDownloads;
    final int downloadCount;

    ShareInfo(JSONObject json) {
        id = json.optString("id");
        fileName = json.optString("file_name", json.optString("file_id"));
        status = json.optString("status");
        expiresAt = json.optString("expires_at");
        maxDownloads = json.optInt("max_downloads");
        downloadCount = json.optInt("download_count");
    }
}
