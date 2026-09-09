package com.sharedisk.lan;

import org.json.JSONObject;

final class FileReplicaInfo {
    final String deviceId;
    final String deviceName;
    final String platform;
    final String state;
    final boolean online;
    final boolean origin;
    final String lastSeenAt;

    FileReplicaInfo(JSONObject json) {
        deviceId = json.optString("device_id");
        deviceName = json.optString("device_name", "未命名设备");
        platform = json.optString("platform");
        state = json.optString("state", "ready");
        online = json.optBoolean("online", false);
        origin = json.optBoolean("is_origin", false);
        lastSeenAt = json.optString("last_seen_at");
    }
}
