import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:image_picker/image_picker.dart';

import '../l10n/app_localizations.dart';
import '../theme/app_theme.dart';

/// Lightweight on-device photo capture gallery. Captures live in session memory
/// for now; wiring uploads to a field asset endpoint is a follow-up (the daily
/// log already references photo_asset_ids).
class PhotosScreen extends ConsumerStatefulWidget {
  const PhotosScreen({super.key});

  @override
  ConsumerState<PhotosScreen> createState() => _PhotosScreenState();
}

class _PhotosScreenState extends ConsumerState<PhotosScreen> {
  final List<String> _photos = [];

  Future<void> _capture() async {
    final shot = await ImagePicker().pickImage(
      source: ImageSource.camera,
      maxWidth: 2048,
      imageQuality: 80,
    );
    if (shot != null) setState(() => _photos.add(shot.path));
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);

    return Scaffold(
      body: _photos.isEmpty
          ? Center(
              child: Text(
                l10n.photosEmpty,
                style: const TextStyle(color: FbColors.textSecondary),
              ),
            )
          : GridView.builder(
              padding: const EdgeInsets.all(FbSizes.gapSmall),
              gridDelegate: const SliverGridDelegateWithFixedCrossAxisCount(
                crossAxisCount: 3,
                mainAxisSpacing: 4,
                crossAxisSpacing: 4,
              ),
              itemCount: _photos.length,
              itemBuilder: (_, i) => ClipRRect(
                borderRadius: BorderRadius.circular(FbSizes.radius),
                child: Image.file(File(_photos[i]), fit: BoxFit.cover),
              ),
            ),
      floatingActionButton: FloatingActionButton.extended(
        onPressed: _capture,
        backgroundColor: FbColors.gableGreen,
        foregroundColor: FbColors.deepSpace,
        icon: const Icon(Icons.photo_camera),
        label: Text(l10n.capturePhoto),
      ),
    );
  }
}
