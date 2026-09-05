package com.sharedisk.lan;

import org.json.JSONObject;

final class FolderInfo {
    final String id;
    final String parentId;
    final String name;

    FolderInfo(JSONObject json) {
        id = json.optString("id");
        parentId = json.optString("parent_id");
        name = json.optString("name");
    }
}
