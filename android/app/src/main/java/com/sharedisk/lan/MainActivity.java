package com.sharedisk.lan;

import android.app.Activity;
import android.app.AlertDialog;
import android.content.ClipData;
import android.content.ClipboardManager;
import android.content.Context;
import android.content.Intent;
import android.graphics.Typeface;
import android.net.Uri;
import android.os.Bundle;
import android.view.View;
import android.widget.Button;
import android.widget.ArrayAdapter;
import android.widget.EditText;
import android.widget.LinearLayout;
import android.widget.PopupMenu;
import android.widget.TextView;
import android.widget.Spinner;

import androidx.work.WorkInfo;
import androidx.work.WorkManager;

import java.util.List;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.Locale;
import java.util.Map;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

public final class MainActivity extends Activity {
    private static final int PICK_UPLOAD = 1001;
    private static final int CREATE_DOWNLOAD = 1002;

    private final ExecutorService executor = Executors.newSingleThreadExecutor();
    private ApiClient api;
    private EditText controlUrl;
    private EditText agentUrl;
    private EditText account;
    private EditText password;
    private EditText bootstrapToken;
    private TextView status;
    private LinearLayout fileList;
    private LinearLayout trashList;
    private LinearLayout discoveryList;
	private LinearLayout transferList;
	private LinearLayout deviceList;
	private LinearLayout folderList;
	private LinearLayout shareList;
	private LinearLayout backgroundDownloadList;
	private EditText searchQuery;
	private Spinner sortSpinner;
	private Button createFolderButton;
	private String selectedFolderId = "";
    private Button bootstrapButton;
    private Button loginButton;
    private Button logoutButton;
    private Button uploadButton;
    private Button refreshButton;
    private Button discoverButton;
    private Button settingsButton;
    private Button filesTab;
    private Button transfersTab;
    private Button devicesTab;
    private Button trashTab;
    private LinearLayout connectionPanel;
    private LinearLayout filesSection;
    private LinearLayout transfersSection;
    private LinearLayout devicesSection;
    private LinearLayout trashSection;
    private LanFile pendingDownload;
    private AgentDiscovery discovery;
    private final Map<String, String> discoveredAgents = new LinkedHashMap<>();

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        setContentView(R.layout.activity_main);
        api = new ApiClient(this);

        controlUrl = findViewById(R.id.controlUrl);
        agentUrl = findViewById(R.id.agentUrl);
        account = findViewById(R.id.account);
        password = findViewById(R.id.password);
        bootstrapToken = findViewById(R.id.bootstrapToken);
        status = findViewById(R.id.status);
        fileList = findViewById(R.id.fileList);
        trashList = findViewById(R.id.trashList);
        discoveryList = findViewById(R.id.discoveryList);
		transferList = findViewById(R.id.transferList);
		deviceList = findViewById(R.id.deviceList);
		folderList = findViewById(R.id.folderList);
		shareList = findViewById(R.id.shareList);
		backgroundDownloadList = findViewById(R.id.backgroundDownloadList);
		searchQuery = findViewById(R.id.searchQuery);
		sortSpinner = findViewById(R.id.sortSpinner);
		createFolderButton = findViewById(R.id.createFolderButton);
		sortSpinner.setAdapter(new ArrayAdapter<>(this, android.R.layout.simple_spinner_dropdown_item, new String[]{"名称升序", "名称降序", "最新", "最早", "大小降序", "大小升序"}));
        bootstrapButton = findViewById(R.id.bootstrapButton);
        loginButton = findViewById(R.id.loginButton);
        logoutButton = findViewById(R.id.logoutButton);
        uploadButton = findViewById(R.id.uploadButton);
        refreshButton = findViewById(R.id.refreshButton);
        discoverButton = findViewById(R.id.discoverButton);
        settingsButton = findViewById(R.id.settingsButton);
        filesTab = findViewById(R.id.filesTab);
        transfersTab = findViewById(R.id.transfersTab);
        devicesTab = findViewById(R.id.devicesTab);
        trashTab = findViewById(R.id.trashTab);
        connectionPanel = findViewById(R.id.connectionPanel);
        filesSection = findViewById(R.id.filesSection);
        transfersSection = findViewById(R.id.transfersSection);
        devicesSection = findViewById(R.id.devicesSection);
        trashSection = findViewById(R.id.trashSection);
        discovery = new AgentDiscovery(this, new AgentDiscovery.Listener() {
            @Override public void onAgentFound(String name, String url) {
                runOnUiThread(() -> addDiscoveredAgent(name, url));
            }

            @Override public void onDiscoveryStatus(String message) {
                runOnUiThread(() -> setStatus(message));
            }
        });

        controlUrl.setText(api.saved("control_url"));
        agentUrl.setText(api.saved("agent_url"));
        account.setText(api.saved("account"));

        bootstrapButton.setOnClickListener(v -> authenticate(true));
        loginButton.setOnClickListener(v -> authenticate(false));
        logoutButton.setOnClickListener(v -> runTask("正在注销…", () -> {
            api.logout();
            runOnUiThread(() -> connectionPanel.setVisibility(View.VISIBLE));
            return "已注销当前会话";
        }, false));
        uploadButton.setOnClickListener(v -> chooseUpload());
        refreshButton.setOnClickListener(v -> refreshFiles());
		createFolderButton.setOnClickListener(v -> promptCreateFolder());
        discoverButton.setOnClickListener(v -> {
            discoveredAgents.clear();
            discoveryList.removeAllViews();
            discovery.start();
        });
        settingsButton.setOnClickListener(v -> connectionPanel.setVisibility(
                connectionPanel.getVisibility() == View.VISIBLE ? View.GONE : View.VISIBLE));
        filesTab.setOnClickListener(v -> showSection(filesSection, filesTab));
        transfersTab.setOnClickListener(v -> showSection(transfersSection, transfersTab));
        devicesTab.setOnClickListener(v -> showSection(devicesSection, devicesTab));
        trashTab.setOnClickListener(v -> showSection(trashSection, trashTab));
        showSection(filesSection, filesTab);

        if (api.hasSession()) {
            connectionPanel.setVisibility(View.GONE);
            setStatus("已有登录会话，正在读取 Ubuntu 文件…");
            refreshFiles();
        } else {
            connectionPanel.setVisibility(View.VISIBLE);
        }
    }

    private void authenticate(boolean bootstrap) {
        saveEndpoints();
        String accountValue = account.getText().toString().trim();
        String passwordValue = password.getText().toString();
        String control = controlUrl.getText().toString().trim();
        if (control.isEmpty() || accountValue.isEmpty() || passwordValue.isEmpty()) {
            setStatus("请填写控制服务地址、账号和密码");
            return;
        }
        runTask(bootstrap ? "正在初始化账号…" : "正在登录…", () -> {
            if (bootstrap) {
                String token = bootstrapToken.getText().toString().trim();
                if (token.isEmpty()) throw new IllegalArgumentException("首次初始化令牌不能为空");
                api.bootstrap(control, accountValue, passwordValue, token);
            } else {
                api.login(control, accountValue, passwordValue);
            }
			api.provisionAgent();
            runOnUiThread(() -> {
                password.setText("");
                bootstrapToken.setText("");
                connectionPanel.setVisibility(View.GONE);
            });
            return "登录成功，正在读取文件…";
        }, true);
    }

    private void chooseUpload() {
        saveEndpoints();
        Intent intent = new Intent(Intent.ACTION_OPEN_DOCUMENT);
        intent.addCategory(Intent.CATEGORY_OPENABLE);
        intent.setType("*/*");
        startActivityForResult(intent, PICK_UPLOAD);
    }

    private void chooseDownload(LanFile file) {
        pendingDownload = file;
        Intent intent = new Intent(Intent.ACTION_CREATE_DOCUMENT);
        intent.addCategory(Intent.CATEGORY_OPENABLE);
        intent.setType(file.mime);
        intent.putExtra(Intent.EXTRA_TITLE, file.name);
        startActivityForResult(intent, CREATE_DOWNLOAD);
    }

    @Override
    protected void onActivityResult(int requestCode, int resultCode, Intent data) {
        super.onActivityResult(requestCode, resultCode, data);
        if (resultCode != RESULT_OK || data == null || data.getData() == null) return;
        Uri uri = data.getData();
        if (requestCode == PICK_UPLOAD) {
            try {
                getContentResolver().takePersistableUriPermission(uri, Intent.FLAG_GRANT_READ_URI_PERMISSION);
            } catch (SecurityException ignored) {
                // Some document providers grant access only for the current process.
            }
            java.util.UUID workID = UploadWorker.enqueue(this, uri);
            setStatus("后台上传已入队\n任务 " + workID);
            refreshFiles();
        } else if (requestCode == CREATE_DOWNLOAD && pendingDownload != null) {
            LanFile file = pendingDownload;
            pendingDownload = null;
			try {
				getContentResolver().takePersistableUriPermission(uri, Intent.FLAG_GRANT_WRITE_URI_PERMISSION);
			} catch (SecurityException ignored) {
				// Worker may still use the provider's current grant until process death.
			}
			java.util.UUID workID = DownloadWorker.enqueue(this, file, uri);
			setStatus("后台下载已入队：" + file.name + "\n任务 " + workID);
			refreshFiles();
        }
    }

    private void refreshFiles() {
        saveEndpoints();
		String folderFilter = selectedFolderId;
		String queryFilter = searchQuery.getText().toString();
		String sortOrder = selectedSort();
        runTask("正在刷新文件列表…", () -> {
			api.provisionAgent();
			List<LanFile> files = api.listFiles(folderFilter, queryFilter, sortOrder);
            List<LanFile> trash = api.listTrash();
			List<TransferInfo> transfers = api.listTransfers();
			List<DeviceInfo> devices = api.listDevices();
			List<FolderInfo> folders = api.listFolders();
			List<ShareInfo> shares = api.listShares();
			String coordinator = api.coordinatorSummary();
			List<WorkInfo> backgroundTransfers = new ArrayList<>();
			backgroundTransfers.addAll(WorkManager.getInstance(this).getWorkInfosByTag(UploadWorker.TAG).get());
			backgroundTransfers.addAll(WorkManager.getInstance(this).getWorkInfosByTag(DownloadWorker.TAG).get());
            runOnUiThread(() -> {
                renderFiles(files);
                renderTrash(trash);
				renderTransfers(transfers);
				renderDevices(devices);
				renderFolders(folders);
				renderShares(shares);
				renderBackgroundDownloads(backgroundTransfers);
            });
            return "已连接：" + files.size() + " 个文件，回收站 " + trash.size() + " 项，" + coordinator;
        }, false);
    }

	private void renderShares(List<ShareInfo> shares) {
		shareList.removeAllViews();
		if (shares.isEmpty()) {
			TextView empty = new TextView(this);
			empty.setText("暂无分享记录");
			shareList.addView(empty);
			return;
		}
		for (ShareInfo share : shares) {
			LinearLayout row = new LinearLayout(this);
			row.setOrientation(LinearLayout.VERTICAL);
			row.setBackgroundResource(R.drawable.panel);
			TextView detail = new TextView(this);
			String limit = share.maxDownloads == 0 ? "不限次数" : share.downloadCount + "/" + share.maxDownloads + " 次";
			detail.setText(share.fileName + " · " + share.status + "\n有效期至 " + displayTime(share.expiresAt) + " · " + limit);
			row.addView(detail);
			if (share.status.equals("active")) {
				Button revoke = new Button(this);
				revoke.setText("撤销分享");
				revoke.setOnClickListener(v -> runTask("正在撤销分享…", () -> {
					api.revokeShare(share);
					return "分享已撤销：" + share.fileName;
				}, true));
				row.addView(revoke);
			}
			shareList.addView(row);
		}
	}

	private void renderBackgroundDownloads(List<WorkInfo> work) {
		backgroundDownloadList.removeAllViews();
		if (work.isEmpty()) {
			TextView empty = new TextView(this);
			empty.setText("暂无后台传输任务");
			empty.setTextColor(getColor(R.color.muted));
			backgroundDownloadList.addView(empty);
			return;
		}
		int start = Math.max(0, work.size() - 10);
		for (int i = start; i < work.size(); i++) {
			WorkInfo item = work.get(i);
			LinearLayout card = new LinearLayout(this);
			card.setOrientation(LinearLayout.VERTICAL);
			card.setBackgroundResource(R.drawable.panel);
			String message = item.getProgress().getString("message");
			if (item.getState().isFinished()) message = item.getOutputData().getString("message");
			TextView detail = new TextView(this);
			detail.setText(item.getState() + (message == null || message.isEmpty() ? "" : " · " + message));
			card.addView(detail);
			if (!item.getState().isFinished()) {
				Button cancel = new Button(this);
				cancel.setAllCaps(false);
				cancel.setText("取消传输");
				cancel.setOnClickListener(v -> {
					WorkManager.getInstance(this).cancelWorkById(item.getId());
					setStatus("已请求取消后台传输");
					refreshFiles();
				});
				card.addView(cancel);
			}
			backgroundDownloadList.addView(card);
		}
	}

	private String selectedSort() {
		switch (sortSpinner.getSelectedItemPosition()) {
		case 1: return "name_desc";
		case 2: return "newest";
		case 3: return "oldest";
		case 4: return "size_desc";
		case 5: return "size_asc";
		default: return "name_asc";
		}
	}

	private void renderFolders(List<FolderInfo> folders) {
		folderList.removeAllViews();
		Button all = new Button(this);
		all.setText(selectedFolderId.isEmpty() ? "✓ 全部文件" : "全部文件");
		all.setAllCaps(false);
		all.setOnClickListener(v -> { selectedFolderId = ""; refreshFiles(); });
		folderList.addView(all);
		for (FolderInfo folder : folders) {
			Button choice = new Button(this);
			choice.setText((folder.id.equals(selectedFolderId) ? "✓ " : "") + folder.name);
			choice.setAllCaps(false);
			choice.setOnClickListener(v -> { selectedFolderId = folder.id; refreshFiles(); });
			folderList.addView(choice);
		}
	}

	private void promptCreateFolder() {
		EditText input = new EditText(this);
		input.setHint("文件夹名称");
		new AlertDialog.Builder(this).setTitle("新建文件夹").setView(input).setNegativeButton("取消", null)
				.setPositiveButton("创建", (dialog, which) -> runTask("正在创建文件夹…", () -> {
					FolderInfo folder = api.createFolder(selectedFolderId, input.getText().toString());
					selectedFolderId = folder.id;
					return "已创建 " + folder.name;
				}, true)).show();
	}

	private void renderTransfers(List<TransferInfo> transfers) {
		transferList.removeAllViews();
		if (transfers.isEmpty()) {
			TextView empty = new TextView(this);
			empty.setText("暂无跨设备复制任务");
			empty.setTextColor(getColor(R.color.muted));
			transferList.addView(empty);
			return;
		}
		for (TransferInfo transfer : transfers) {
			LinearLayout card = new LinearLayout(this);
			card.setOrientation(LinearLayout.VERTICAL);
			card.setBackgroundResource(R.drawable.panel);
			TextView detail = new TextView(this);
			detail.setText(transfer.state + " · 尝试 " + transfer.attempt + "\n" + prefix(transfer.objectId, 20));
			card.addView(detail);
			if (!transfer.terminal()) {
				Button cancel = new Button(this);
				cancel.setText("取消任务");
				cancel.setOnClickListener(v -> runTask("正在取消任务…", () -> {
					api.cancelTransfer(transfer);
					return "任务已取消";
				}, true));
				card.addView(cancel);
			}
			transferList.addView(card);
		}
	}

	private void renderDevices(List<DeviceInfo> devices) {
		deviceList.removeAllViews();
		for (DeviceInfo device : devices) {
			LinearLayout row = new LinearLayout(this);
			row.setOrientation(LinearLayout.VERTICAL);
			row.setBackgroundResource(R.drawable.panel);
			TextView detail = new TextView(this);
			detail.setText(device.name + " · " + device.platform + "\n" + (device.lastSeenAt.isEmpty() ? "尚无心跳" : displayTime(device.lastSeenAt)));
			row.addView(detail);
			if (!device.id.equals(api.currentDeviceId())) {
				Button remove = new Button(this);
				remove.setText("移除设备");
				remove.setOnClickListener(v -> new AlertDialog.Builder(this)
						.setTitle("移除设备？")
						.setMessage(device.name + " 的现有会话将失效")
						.setNegativeButton("取消", null)
						.setPositiveButton("移除", (dialog, which) -> runTask("正在移除设备…", () -> {
							api.deregisterDevice(device);
							return "已移除 " + device.name;
						}, true)).show());
				row.addView(remove);
			}
			deviceList.addView(row);
		}
	}

    private void addDiscoveredAgent(String name, String url) {
        if (discoveredAgents.put(url, name) != null) return;
        Button choice = new Button(this);
        choice.setAllCaps(false);
        choice.setText(name + "\n" + url);
        choice.setOnClickListener(v -> {
            agentUrl.setText(url);
            saveEndpoints();
            setStatus("已选择 " + name + "，登录后可访问；自动发现本身不授予权限");
            discovery.stop();
        });
        discoveryList.addView(choice);
    }

    private void renderFiles(List<LanFile> files) {
        fileList.removeAllViews();
        if (files.isEmpty()) {
            TextView empty = new TextView(this);
            empty.setText("还没有文件。可从 Android 选择文件上传。\nUbuntu 也可通过 LAN API 上传后在这里刷新。");
            empty.setTextColor(getColor(R.color.muted));
            empty.setPadding(dp(4), dp(12), dp(4), dp(20));
            fileList.addView(empty);
            return;
        }
        for (LanFile file : files) {
            LinearLayout card = new LinearLayout(this);
            card.setOrientation(LinearLayout.VERTICAL);
            card.setBackgroundResource(R.drawable.panel);
            LinearLayout.LayoutParams params = new LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT);
            params.bottomMargin = dp(10);
            card.setLayoutParams(params);

            TextView title = new TextView(this);
            title.setText(file.name);
            title.setTextColor(getColor(R.color.ink));
            title.setTextSize(17);
            title.setTypeface(Typeface.DEFAULT, Typeface.BOLD);
            card.addView(title);

            TextView details = new TextView(this);
			details.setText(String.format(Locale.US, "%,d 字节  ·  %s\n%s…  ·  %s", file.size, file.mime, prefix(file.sha256, 16), file.originDeviceName));
            details.setTextColor(getColor(R.color.muted));
            details.setTextSize(13);
            details.setPadding(0, dp(5), 0, dp(8));
            card.addView(details);

            LinearLayout actions = new LinearLayout(this);
            actions.setOrientation(LinearLayout.HORIZONTAL);

            Button download = new Button(this);
            download.setText("下载并校验");
            download.setAllCaps(false);
            download.setOnClickListener(v -> chooseDownload(file));
            actions.addView(download, new LinearLayout.LayoutParams(0, dp(48), 1));

            Button more = new Button(this);
            more.setText("更多操作");
            more.setAllCaps(false);
            more.setOnClickListener(v -> showFileActions(v, file));
            LinearLayout.LayoutParams moreParams = new LinearLayout.LayoutParams(0, dp(48), 1);
            moreParams.leftMargin = dp(8);
            actions.addView(more, moreParams);
            card.addView(actions);
            fileList.addView(card);
        }
    }

    private void showFileActions(View anchor, LanFile file) {
        PopupMenu menu = new PopupMenu(this, anchor);
        menu.getMenu().add("重命名");
        menu.getMenu().add("移动到文件夹");
        menu.getMenu().add("复制到另一台 Ubuntu");
        menu.getMenu().add("创建 24 小时分享");
        menu.getMenu().add("移到回收站");
        menu.setOnMenuItemClickListener(item -> {
            switch (item.getTitle().toString()) {
                case "重命名": promptRename(file); break;
                case "移动到文件夹": promptMove(file); break;
                case "复制到另一台 Ubuntu": promptReplication(file); break;
                case "创建 24 小时分享": createShare(file); break;
                case "移到回收站": confirmTrash(file); break;
                default: return false;
            }
            return true;
        });
        menu.show();
    }

    private void showSection(LinearLayout selected, Button selectedTab) {
        filesSection.setVisibility(selected == filesSection ? View.VISIBLE : View.GONE);
        transfersSection.setVisibility(selected == transfersSection ? View.VISIBLE : View.GONE);
        devicesSection.setVisibility(selected == devicesSection ? View.VISIBLE : View.GONE);
        trashSection.setVisibility(selected == trashSection ? View.VISIBLE : View.GONE);
        filesTab.setEnabled(selectedTab != filesTab);
        transfersTab.setEnabled(selectedTab != transfersTab);
        devicesTab.setEnabled(selectedTab != devicesTab);
        trashTab.setEnabled(selectedTab != trashTab);
    }

    private void createShare(LanFile file) {
        runTask("正在创建分享链接…", () -> {
            String link = api.createShare(file);
            runOnUiThread(() -> {
                TextView value = new TextView(this);
                value.setText(link);
                value.setTextIsSelectable(true);
                value.setPadding(dp(20), dp(12), dp(20), dp(12));
                new AlertDialog.Builder(this)
                        .setTitle("分享链接已创建")
                        .setMessage("链接有效 24 小时。访问后会得到文件信息、Agent 下载地址和短期下载令牌。")
                        .setView(value)
                        .setNegativeButton("关闭", null)
                        .setPositiveButton("复制", (dialog, which) -> {
                            ClipboardManager clipboard = (ClipboardManager) getSystemService(Context.CLIPBOARD_SERVICE);
                            clipboard.setPrimaryClip(ClipData.newPlainText("Share Disk link", link));
                            setStatus("分享链接已复制");
                        }).show();
            });
            return "分享链接已创建";
        }, false);
    }

	private void promptMove(LanFile file) {
		setBusy(true);
		executor.execute(() -> {
			try {
				List<FolderInfo> folders = api.listFolders();
				runOnUiThread(() -> {
					setBusy(false);
					String[] names = new String[folders.size()];
					for (int i = 0; i < folders.size(); i++) names[i] = folders.get(i).name;
					new AlertDialog.Builder(this).setTitle("移动到").setItems(names, (dialog, which) -> runTask("正在移动…", () -> {
						api.moveFile(file, folders.get(which));
						return "已移动 " + file.name;
					}, true)).setNegativeButton("取消", null).show();
				});
			} catch (Exception error) {
				runOnUiThread(() -> { setBusy(false); setStatus("读取文件夹失败：" + safeMessage(error)); });
			}
		});
	}

    private void promptReplication(LanFile file) {
        setBusy(true);
        setStatus("正在读取可用设备…");
        executor.execute(() -> {
            try {
                List<DeviceInfo> all = api.listDevices();
                List<DeviceInfo> targets = new java.util.ArrayList<>();
                for (DeviceInfo device : all) {
                    if (device.platform.equalsIgnoreCase("linux") && !device.id.equals(file.originDeviceId)) {
                        targets.add(device);
                    }
                }
                runOnUiThread(() -> {
                    setBusy(false);
                    if (targets.isEmpty()) {
                        setStatus("没有其他已注册的 Ubuntu Agent 可作为复制目标");
                        return;
                    }
                    String[] labels = new String[targets.size()];
                    for (int i = 0; i < targets.size(); i++) {
                        DeviceInfo device = targets.get(i);
                        labels[i] = device.name + (device.lastSeenAt.isEmpty() ? " · 尚无心跳" : " · 最近在线 " + displayTime(device.lastSeenAt));
                    }
                    new AlertDialog.Builder(this)
                            .setTitle("选择复制目标")
                            .setItems(labels, (dialog, which) -> {
                                DeviceInfo target = targets.get(which);
                                runTask("正在提交复制任务…", () -> {
                                    api.scheduleReplica(file, target);
                                    return "已提交到 " + target.name + "；目标 Agent 将领取、校验并登记副本";
                                }, false);
                            })
                            .setNegativeButton("取消", null)
                            .show();
                    setStatus("请选择目标 Ubuntu Agent");
                });
            } catch (Exception error) {
                runOnUiThread(() -> {
                    setBusy(false);
                    setStatus("读取设备失败：" + safeMessage(error));
                });
            }
        });
    }

    private void renderTrash(List<LanFile> files) {
        trashList.removeAllViews();
        if (files.isEmpty()) {
            TextView empty = new TextView(this);
            empty.setText("回收站为空");
            empty.setTextColor(getColor(R.color.muted));
            empty.setPadding(dp(4), dp(12), dp(4), dp(20));
            trashList.addView(empty);
            return;
        }
        for (LanFile file : files) {
            LinearLayout card = new LinearLayout(this);
            card.setOrientation(LinearLayout.VERTICAL);
            card.setBackgroundResource(R.drawable.panel);
            LinearLayout.LayoutParams params = new LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT);
            params.bottomMargin = dp(10);
            card.setLayoutParams(params);

            TextView title = new TextView(this);
            title.setText(file.name);
            title.setTextColor(getColor(R.color.ink));
            title.setTextSize(17);
            title.setTypeface(Typeface.DEFAULT, Typeface.BOLD);
            card.addView(title);

            TextView details = new TextView(this);
            details.setText("删除时间 " + displayTime(file.deletedAt) + "\n自动清理 " + displayTime(file.purgeAfter));
            details.setTextColor(getColor(R.color.muted));
            details.setPadding(0, dp(5), 0, dp(8));
            card.addView(details);

            Button restore = new Button(this);
            restore.setText("恢复");
            restore.setEnabled(file.status.equals("trashed"));
            restore.setOnClickListener(v -> runTask("正在恢复…", () -> {
                api.restoreFile(file);
                return "已恢复 " + file.name;
            }, true));
            card.addView(restore);

            Button purge = new Button(this);
            purge.setText("永久删除");
            purge.setOnClickListener(v -> confirmPurge(file));
            card.addView(purge);
            trashList.addView(card);
        }
    }

    private void promptRename(LanFile file) {
        EditText input = new EditText(this);
        input.setSingleLine(true);
        input.setText(file.name);
        input.selectAll();
        new AlertDialog.Builder(this)
                .setTitle("重命名")
                .setView(input)
                .setNegativeButton("取消", null)
                .setPositiveButton("保存", (dialog, which) -> runTask("正在重命名…", () -> {
                    LanFile renamed = api.renameFile(file, input.getText().toString());
                    return "已重命名为 " + renamed.name;
                }, true))
                .show();
    }

    private void confirmTrash(LanFile file) {
		new AlertDialog.Builder(this)
				.setTitle("移到回收站？")
				.setMessage(file.name + " 可在回收站显示的自动清理时间前恢复。")
                .setNegativeButton("取消", null)
                .setPositiveButton("移到回收站", (dialog, which) -> runTask("正在删除…", () -> {
                    api.trashFile(file);
                    return "已移到回收站：" + file.name;
                }, true))
                .show();
    }

    private void confirmPurge(LanFile file) {
        new AlertDialog.Builder(this)
                .setTitle("永久删除？")
                .setMessage("此操作无法恢复。若没有其他文件引用相同内容，Ubuntu 上的正文也会被删除。")
                .setNegativeButton("取消", null)
                .setPositiveButton("永久删除", (dialog, which) -> runTask("正在永久删除…", () -> {
                    api.purgeFile(file);
                    return "已永久删除：" + file.name;
                }, true))
                .show();
    }

    private static String displayTime(String value) {
        if (value == null || value.isEmpty()) return "未知";
        return value.replace('T', ' ').replace("Z", " UTC");
    }

    private interface Task {
        String run() throws Exception;
    }

    private void runTask(String starting, Task task, boolean refreshAfter) {
        setBusy(true);
        setStatus(starting);
        executor.execute(() -> {
            try {
                String message = task.run();
                runOnUiThread(() -> {
                    setBusy(false);
                    setStatus(message);
                    if (refreshAfter) refreshFiles();
                });
            } catch (Exception error) {
                runOnUiThread(() -> {
                    setBusy(false);
                    setStatus("操作失败：" + safeMessage(error));
                });
            }
        });
    }

    private void saveEndpoints() {
        api.saveEndpoints(controlUrl.getText().toString(), agentUrl.getText().toString(), account.getText().toString());
    }

    private void setBusy(boolean busy) {
        bootstrapButton.setEnabled(!busy);
        loginButton.setEnabled(!busy);
        logoutButton.setEnabled(!busy);
        uploadButton.setEnabled(!busy);
        refreshButton.setEnabled(!busy);
        discoverButton.setEnabled(!busy);
		createFolderButton.setEnabled(!busy);
    }

    private void setStatusFromWorker(String value) {
        runOnUiThread(() -> setStatus(value));
    }

    private void setStatus(String value) {
        status.setText(value);
    }

    private int dp(int value) {
        return Math.round(value * getResources().getDisplayMetrics().density);
    }

    private static String safeMessage(Exception error) {
        String message = error.getMessage();
        return message == null || message.isEmpty() ? error.getClass().getSimpleName() : message;
    }

    private static String prefix(String value, int length) {
        return value.length() <= length ? value : value.substring(0, length);
    }

    @Override
    protected void onDestroy() {
        discovery.stop();
        executor.shutdownNow();
        super.onDestroy();
    }
}
