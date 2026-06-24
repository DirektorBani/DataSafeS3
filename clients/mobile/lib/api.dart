import 'dart:convert';
import 'package:http/http.dart' as http;

class DatasafeApi {
  DatasafeApi(this.baseUrl, this.token);

  final String baseUrl;
  String token;

  Uri _u(String path) => Uri.parse('${baseUrl.replaceAll(RegExp(r'/+$'), '')}$path');

  Map<String, String> get _headers => {
        'Authorization': 'Bearer $token',
        'Content-Type': 'application/json',
      };

  Future<void> login(String username, String password) async {
    final res = await http.post(
      _u('/api/v1/admin/login'),
      headers: {'Content-Type': 'application/json'},
      body: jsonEncode({'username': username, 'password': password}),
    );
    if (res.statusCode != 200) {
      throw Exception('login failed: ${res.body}');
    }
    final body = jsonDecode(res.body) as Map<String, dynamic>;
    if (body['mfa_required'] == true) {
      throw Exception('MFA required — use mobile-web or console');
    }
    token = body['token'] as String;
  }

  Future<List<BucketRow>> listBuckets() async {
    final res = await http.get(_u('/api/v1/buckets?filter=all'), headers: _headers);
    if (res.statusCode != 200) throw Exception(res.body);
    final list = (jsonDecode(res.body)['buckets'] as List).cast<Map<String, dynamic>>();
    return list.map((b) => BucketRow(name: b['name'] as String)).toList();
  }

  Future<List<ObjectRow>> listObjects(String bucket, {String prefix = ''}) async {
    final q = prefix.isEmpty ? '' : '?prefix=${Uri.encodeComponent(prefix)}';
    final res = await http.get(_u('/api/v1/buckets/${Uri.encodeComponent(bucket)}/objects$q'), headers: _headers);
    if (res.statusCode != 200) throw Exception(res.body);
    final objs = (jsonDecode(res.body)['objects'] as List).cast<Map<String, dynamic>>();
    return objs
        .map((o) => ObjectRow(key: o['key'] as String, size: (o['size'] as num).toInt()))
        .where((o) => !o.key.endsWith('/') || o.size > 0)
        .toList();
  }

  Future<void> uploadBytes(String bucket, String key, List<int> bytes, {String contentType = 'application/octet-stream'}) async {
    final segments = key.split('/').map(Uri.encodeComponent).join('/');
    final res = await http.put(
      _u('/api/v1/buckets/${Uri.encodeComponent(bucket)}/objects/$segments'),
      headers: {..._headers, 'Content-Type': contentType},
      body: bytes,
    );
    if (res.statusCode != 200 && res.statusCode != 201) {
      throw Exception('upload ${res.statusCode}: ${res.body}');
    }
  }
}

class BucketRow {
  BucketRow({required this.name});
  final String name;
}

class ObjectRow {
  ObjectRow({required this.key, required this.size});
  final String key;
  final int size;
}
