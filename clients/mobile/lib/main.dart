import 'package:flutter/material.dart';
import 'package:file_picker/file_picker.dart';
import 'api.dart';

void main() => runApp(const DatasafeMobileApp());

class DatasafeMobileApp extends StatelessWidget {
  const DatasafeMobileApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'DataSafeS3',
      theme: ThemeData(colorSchemeSeed: const Color(0xFF2563EB), useMaterial3: true),
      home: const LoginScreen(),
    );
  }
}

class LoginScreen extends StatefulWidget {
  const LoginScreen({super.key});

  @override
  State<LoginScreen> createState() => _LoginScreenState();
}

class _LoginScreenState extends State<LoginScreen> {
  final _server = TextEditingController(text: 'http://10.0.2.2:8080');
  final _user = TextEditingController();
  final _pass = TextEditingController();
  bool _loading = false;
  String? _error;

  Future<void> _login() async {
    setState(() { _loading = true; _error = null; });
    try {
      final api = DatasafeApi(_server.text.trim(), '');
      await api.login(_user.text.trim(), _pass.text);
      if (!mounted) return;
      Navigator.of(context).pushReplacement(
        MaterialPageRoute(builder: (_) => FilesScreen(api: api)),
      );
    } catch (e) {
      setState(() => _error = e.toString());
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('DataSafeS3')),
      body: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            TextField(controller: _server, decoration: const InputDecoration(labelText: 'Server URL')),
            TextField(controller: _user, decoration: const InputDecoration(labelText: 'Username')),
            TextField(controller: _pass, obscureText: true, decoration: const InputDecoration(labelText: 'Password')),
            if (_error != null) Text(_error!, style: const TextStyle(color: Colors.red)),
            const SizedBox(height: 12),
            FilledButton(onPressed: _loading ? null : _login, child: Text(_loading ? 'Signing in…' : 'Sign in')),
          ],
        ),
      ),
    );
  }
}

class FilesScreen extends StatefulWidget {
  const FilesScreen({super.key, required this.api});
  final DatasafeApi api;

  @override
  State<FilesScreen> createState() => _FilesScreenState();
}

class _FilesScreenState extends State<FilesScreen> {
  List<BucketRow> _buckets = [];
  String? _bucket;
  List<ObjectRow> _objects = [];
  bool _loading = true;

  @override
  void initState() {
    super.initState();
    _loadBuckets();
  }

  Future<void> _loadBuckets() async {
    setState(() => _loading = true);
    try {
      _buckets = await widget.api.listBuckets();
      _bucket ??= _buckets.isNotEmpty ? _buckets.first.name : null;
      if (_bucket != null) await _loadObjects();
    } catch (e) {
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('$e')));
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _loadObjects() async {
    if (_bucket == null) return;
    _objects = await widget.api.listObjects(_bucket!);
    setState(() {});
  }

  Future<void> _upload() async {
    if (_bucket == null) return;
    final pick = await FilePicker.platform.pickFiles(withData: true);
    if (pick == null || pick.files.isEmpty) return;
    final f = pick.files.first;
    final name = f.name;
    await widget.api.uploadBytes(_bucket!, name, f.bytes ?? []);
    await _loadObjects();
    if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('Uploaded $name')));
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: Text(_bucket ?? 'Files'),
        actions: [
          if (_bucket != null)
            IconButton(icon: const Icon(Icons.upload_file), onPressed: _upload),
        ],
      ),
      body: _loading
          ? const Center(child: CircularProgressIndicator())
          : Row(
              children: [
                SizedBox(
                  width: 140,
                  child: ListView(
                    children: _buckets
                        .map((b) => ListTile(
                              title: Text(b.name, style: const TextStyle(fontSize: 14)),
                              selected: b.name == _bucket,
                              onTap: () async {
                                setState(() => _bucket = b.name);
                                await _loadObjects();
                              },
                            ))
                        .toList(),
                  ),
                ),
                const VerticalDivider(width: 1),
                Expanded(
                  child: ListView.builder(
                    itemCount: _objects.length,
                    itemBuilder: (_, i) {
                      final o = _objects[i];
                      return ListTile(
                        leading: const Icon(Icons.insert_drive_file_outlined),
                        title: Text(o.key, maxLines: 2, overflow: TextOverflow.ellipsis),
                        subtitle: Text('${o.size} bytes'),
                      );
                    },
                  ),
                ),
              ],
            ),
    );
  }
}
