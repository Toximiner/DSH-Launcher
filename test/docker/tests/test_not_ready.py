"""«dsh не готов»: порт открыт, но приложение не отвечает (долгий бут) —
лаунчер не оставляет серое окно, а показывает страницу с кнопками
«Проверить снова» / «Перезапустить dsh»; после «Проверить снова» — снова в GUI.
"""
import os

from helpers.env import FAKE, NOUPD
from helpers.procs import fakedsh_events, wait_for


def test_dsh_not_ready(start, gui):
    """dsh перезапустился сам (как после обновления плагина) и долго «бует»:
    порт открыт, а HTTP-сервер не отвечает. Лаунчер дожидается готовности
    (15 с), не дождавшись — страница «dsh не готов»; «Проверить снова»
    (dshlauncher://retry/) — подключение, как «dsh запущен вручную»."""
    hold = '/tmp/hold-http'
    if os.path.exists(hold):
        os.remove(hold)
    launcher = start(FAKE, NOUPD, FAKE_SELF_RESTART_AFTER='3', FAKE_REBORN_HOLD_HTTP=hold)
    gui.wait()
    assert 'FAKE DSH main' in (gui.eval('document.body.innerText') or ''), 'первоначальный GUI'

    # 1) Самоперезапуск: дочкин dsh ушёл, новый поднял тот же порт, но HTTP держит.
    assert launcher.wait_log('порт снова открыт — dsh перезапустился сам', 30), 'переход в режим подключения'
    assert launcher.wait_log('recover: dsh не готов за 15000 мс', 40), '15 с без ответа — «не готов»'
    assert 'http held' in fakedsh_events(), 'reborn держит HTTP-ответы'

    # 2) Страница «dsh не готов» (язык контейнера — en): объяснение + кнопки.
    assert gui.drv.wait_text(r'dsh is not ready.*port 3080 is open.*not responding yet', 10)
    assert gui.drv.wait_text(r'Check again.*Restart dsh', 5), 'кнопки «Проверить снова» и «Перезапустить dsh»'

    # 3) «Проверить снова»: дsh уже отвечает — окно снова в GUI.
    open(hold, 'w').close()
    assert wait_for(lambda: 'http released' in fakedsh_events(), 10), 'dsh «дозагрузился»'
    gui.drv.go('dshlauncher://retry/')
    assert launcher.wait_log('retry: dsh запущен: куки валидны — открываю GUI', 30), 'подключение по retry'
    assert wait_for(lambda: 'FAKE DSH reborn' in (gui.eval('document.body.innerText') or ''), 20), 'GUI перезапущенного dsh'

    launcher.term()
