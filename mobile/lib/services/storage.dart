/// Storage：SharedPreferences 包装，持久化轻量配置。
library;

import 'package:shared_preferences/shared_preferences.dart';

class Storage {
  final SharedPreferences _sp;
  Storage(this._sp);

  static const _kBaseUrl = 'backend.baseUrl';
  static const _kLastServerId = 'last.serverId';
  static const _kLastProjectId = 'last.projectId';

  Future<String> getBaseUrl() async => _sp.getString(_kBaseUrl) ?? '';
  Future<void> setBaseUrl(String url) => _sp.setString(_kBaseUrl, url);

  Future<String?> getLastServerId() async => _sp.getString(_kLastServerId);
  Future<void> setLastServerId(String? id) async {
    if (id == null) {
      await _sp.remove(_kLastServerId);
    } else {
      await _sp.setString(_kLastServerId, id);
    }
  }

  Future<String?> getLastProjectId() async => _sp.getString(_kLastProjectId);
  Future<void> setLastProjectId(String? id) async {
    if (id == null) {
      await _sp.remove(_kLastProjectId);
    } else {
      await _sp.setString(_kLastProjectId, id);
    }
  }
}
