// ignore: unused_import
import 'package:intl/intl.dart' as intl;
import 'app_localizations.dart';

// ignore_for_file: type=lint

/// The translations for English (`en`).
class AppLocalizationsEn extends AppLocalizations {
  AppLocalizationsEn([String locale = 'en']) : super(locale);

  @override
  String get appTitle => 'BuildOS Field';

  @override
  String get navTasks => 'Tasks';

  @override
  String get navLog => 'Log';

  @override
  String get navPhotos => 'Photos';

  @override
  String get navMore => 'More';

  @override
  String get navSync => 'Sync';

  @override
  String get navProfile => 'Profile';

  @override
  String get navSchedule => 'Schedule';

  @override
  String get online => 'Online';

  @override
  String offlineQueued(int count) {
    return 'Offline · $count queued';
  }

  @override
  String get syncing => 'Syncing…';

  @override
  String get loginTitle => 'Sign in to BuildOS';

  @override
  String get loginSubtitle => 'Your construction system of execution.';

  @override
  String get emailLabel => 'Email';

  @override
  String get passwordLabel => 'Password';

  @override
  String get signIn => 'Sign in';

  @override
  String get signOut => 'Sign out';

  @override
  String get offlineSignInDisabled =>
      'You\'re offline — sign in needs a connection.';

  @override
  String get invalidCredentials => 'Email or password is incorrect.';

  @override
  String get sessionExpired => 'Your session expired. Sign in again.';

  @override
  String get genericError => 'Something went wrong. Try again.';

  @override
  String get tasksTitle => 'Tasks';

  @override
  String get tasksEmpty => 'No tasks assigned to you yet.';

  @override
  String get criticalPath => 'Critical path';

  @override
  String floatDays(int days) {
    return '${days}d float';
  }

  @override
  String percentComplete(int pct) {
    return '$pct% complete';
  }

  @override
  String get slideToComplete => 'Slide to complete';

  @override
  String get markDone => 'Mark done';

  @override
  String get queuedForSync => 'Queued — will sync when online.';

  @override
  String get dailyLogTitle => 'Daily Log';

  @override
  String get workSummary => 'Work summary';

  @override
  String get weatherConditions => 'Weather';

  @override
  String get safetyIncidents => 'Safety incidents';

  @override
  String get crewCheckIn => 'Crew check-in';

  @override
  String get capturePhoto => 'Capture photo';

  @override
  String get locationCaptured => 'Location captured';

  @override
  String get locationUnavailable => 'Location unavailable';

  @override
  String get submit => 'Submit';

  @override
  String get photosTitle => 'Photos';

  @override
  String get photosEmpty => 'No photos captured yet.';

  @override
  String get syncStatusTitle => 'Sync Status';

  @override
  String lastSynced(String time) {
    return 'Last synced $time';
  }

  @override
  String get neverSynced => 'Never synced';

  @override
  String get queuedActions => 'Queued actions';

  @override
  String get retryNow => 'Retry now';

  @override
  String get nothingQueued => 'Everything is up to date.';

  @override
  String get profileTitle => 'Profile';

  @override
  String get roleLabel => 'Role';

  @override
  String get languageLabel => 'Language';

  @override
  String get scheduleTitle => 'Schedule';

  @override
  String get scheduleReadOnly => 'Read-only on the field app.';

  @override
  String get loading => 'Loading…';

  @override
  String get retry => 'Retry';

  @override
  String get projectLabel => 'Project';

  @override
  String get crewOnSite => 'Crew on site';

  @override
  String get crewMemberName => 'Name';

  @override
  String get crewMemberRole => 'Role (optional)';

  @override
  String get addCrewMember => 'Add crew member';

  @override
  String get removeCrewMember => 'Remove crew member';

  @override
  String get checkInNotes => 'Notes (optional)';

  @override
  String get updateLocation => 'Update';

  @override
  String get offlineWillQueue =>
      'Offline — this check-in will queue and sync later.';

  @override
  String get submitCheckIn => 'Submit check-in';

  @override
  String get checkInQueued => 'Check-in queued — will sync when online.';

  @override
  String get checkInNeedsCrew => 'Add at least one crew member.';

  @override
  String get notesTooLong => 'Notes are too long. Shorten and try again.';

  @override
  String get equipmentTitle => 'Equipment';

  @override
  String get equipmentEmpty => 'No equipment on your sites yet.';

  @override
  String get equipmentStatusAvailable => 'Available';

  @override
  String get equipmentStatusMaintenance => 'Maintenance';

  @override
  String get equipmentStatusUnavailable => 'Unavailable';

  @override
  String get equipmentOnSite => 'On site';

  @override
  String equipmentSerial(String serial) {
    return 'SN $serial';
  }
}
