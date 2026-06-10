// ignore: unused_import
import 'package:intl/intl.dart' as intl;
import 'app_localizations.dart';

// ignore_for_file: type=lint

/// The translations for Spanish Castilian (`es`).
class AppLocalizationsEs extends AppLocalizations {
  AppLocalizationsEs([String locale = 'es']) : super(locale);

  @override
  String get appTitle => 'BuildOS Campo';

  @override
  String get navTasks => 'Tareas';

  @override
  String get navLog => 'Registro';

  @override
  String get navPhotos => 'Fotos';

  @override
  String get navMore => 'Más';

  @override
  String get navSync => 'Sincronizar';

  @override
  String get navProfile => 'Perfil';

  @override
  String get navSchedule => 'Cronograma';

  @override
  String get online => 'En línea';

  @override
  String offlineQueued(int count) {
    return 'Sin conexión · $count en cola';
  }

  @override
  String get syncing => 'Sincronizando…';

  @override
  String get loginTitle => 'Iniciar sesión en BuildOS';

  @override
  String get loginSubtitle => 'Su sistema de ejecución de construcción.';

  @override
  String get emailLabel => 'Correo electrónico';

  @override
  String get passwordLabel => 'Contraseña';

  @override
  String get signIn => 'Iniciar sesión';

  @override
  String get signOut => 'Cerrar sesión';

  @override
  String get offlineSignInDisabled =>
      'Está sin conexión — iniciar sesión requiere conexión.';

  @override
  String get invalidCredentials => 'El correo o la contraseña son incorrectos.';

  @override
  String get sessionExpired => 'Su sesión expiró. Inicie sesión de nuevo.';

  @override
  String get genericError => 'Algo salió mal. Inténtelo de nuevo.';

  @override
  String get tasksTitle => 'Tareas';

  @override
  String get tasksEmpty => 'Aún no tiene tareas asignadas.';

  @override
  String get criticalPath => 'Ruta crítica';

  @override
  String floatDays(int days) {
    return '${days}d de holgura';
  }

  @override
  String percentComplete(int pct) {
    return '$pct% completado';
  }

  @override
  String get slideToComplete => 'Deslice para completar';

  @override
  String get markDone => 'Marcar como hecho';

  @override
  String get queuedForSync => 'En cola — se sincronizará cuando haya conexión.';

  @override
  String get dailyLogTitle => 'Registro diario';

  @override
  String get workSummary => 'Resumen del trabajo';

  @override
  String get weatherConditions => 'Clima';

  @override
  String get safetyIncidents => 'Incidentes de seguridad';

  @override
  String get crewCheckIn => 'Registro de cuadrilla';

  @override
  String get capturePhoto => 'Tomar foto';

  @override
  String get locationCaptured => 'Ubicación capturada';

  @override
  String get locationUnavailable => 'Ubicación no disponible';

  @override
  String get submit => 'Enviar';

  @override
  String get photosTitle => 'Fotos';

  @override
  String get photosEmpty => 'Aún no se han tomado fotos.';

  @override
  String get syncStatusTitle => 'Estado de sincronización';

  @override
  String lastSynced(String time) {
    return 'Última sincronización $time';
  }

  @override
  String get neverSynced => 'Nunca sincronizado';

  @override
  String get queuedActions => 'Acciones en cola';

  @override
  String get retryNow => 'Reintentar ahora';

  @override
  String get nothingQueued => 'Todo está actualizado.';

  @override
  String get profileTitle => 'Perfil';

  @override
  String get roleLabel => 'Rol';

  @override
  String get languageLabel => 'Idioma';

  @override
  String get scheduleTitle => 'Cronograma';

  @override
  String get scheduleReadOnly => 'Solo lectura en la app de campo.';

  @override
  String get loading => 'Cargando…';

  @override
  String get retry => 'Reintentar';

  @override
  String get projectLabel => 'Proyecto';

  @override
  String get crewOnSite => 'Cuadrilla en el sitio';

  @override
  String get crewMemberName => 'Nombre';

  @override
  String get crewMemberRole => 'Función (opcional)';

  @override
  String get addCrewMember => 'Agregar miembro';

  @override
  String get removeCrewMember => 'Quitar miembro';

  @override
  String get checkInNotes => 'Notas (opcional)';

  @override
  String get updateLocation => 'Actualizar';

  @override
  String get offlineWillQueue =>
      'Sin conexión — este registro se pondrá en cola y se sincronizará después.';

  @override
  String get submitCheckIn => 'Enviar registro';

  @override
  String get checkInQueued =>
      'Registro en cola — se sincronizará al conectarse.';

  @override
  String get checkInNeedsCrew => 'Agregue al menos un miembro de la cuadrilla.';

  @override
  String get notesTooLong =>
      'Las notas son demasiado largas. Acórtelas e intente de nuevo.';
}
