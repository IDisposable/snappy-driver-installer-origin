/*
This file is part of Snappy Driver Installer Origin.

Snappy Driver Installer Origin is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by the Free Software
Foundation, either version 3 of the License or (at your option) any later version.

Snappy Driver Installer Origin is distributed in the hope that it will be useful
but WITHOUT ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
FITNESS FOR A PARTICULAR PURPOSE.  See the GNU General Public License for more details.

You should have received a copy of the GNU General Public License along with
Snappy Driver Installer Origin.  If not, see <http://www.gnu.org/licenses/>.
*/

#include "com_header.h"
#include "common.h"
#include "logging.h"
#include "system.h"     // non-portable
#include "settings.h"
#include "cli.h"
#include "indexing.h"
#include "manager.h"
#include "welcome.h"

#ifdef USE_TORRENT
#include "update.h"
#endif

#include "install.h"    // non-portable
#include "gui.h"
#include "draw.h"   // non-portable
#include "theme.h"
#include "usbwizard.h"
#include "shellapi.h"
#include "commdlg.h"
#include "tchar.h"

#include <windows.h>
#include <setupapi.h>       // for CommandLineToArgvW
#include <shobjidl.h>       // for TBPF_NORMAL
#include <process.h>
#include <signal.h>
#include <iostream>

// Depend on Win32API
#include "enum.h"   // non-portable
#include "main.h"
#include "model.h"
#include "script.h"

#include "welcome.h"

extern HBRUSH g_hbrDlgBackground; // main.cpp

INT_PTR CALLBACK WelcomeProcedure(HWND hwnd,UINT msg,WPARAM wParam,LPARAM lParam)
{
    HWND Ctl1;
    HWND Ctl2;
//    HWND Ctl3;
    HWND Ctl4;

    switch (msg)
    {
    case WM_INITDIALOG:
        // languages
        SetWindowText(GetDlgItem(hwnd,IDD_WELC_TITLE),STR(STR_WELCOME_TITLE));
        SetWindowText(GetDlgItem(hwnd,IDD_WELC_SUBTITLE),STR(STR_WELCOME_SUBTITLE));
        SetWindowText(GetDlgItem(hwnd,IDD_WELC_INTRO),STR(STR_WELCOME_INTRO));
        SetWindowText(GetDlgItem(hwnd,IDD_WELC_INTRO2),STR(STR_WELCOME_INTRO2));
        SetWindowText(GetDlgItem(hwnd,IDD_WELC_BUTTON1),STR(STR_WELCOME_BUTTON1));
        SetWindowText(GetDlgItem(hwnd,IDD_WELC_BUTTON1_DESC),STR(STR_WELCOME_BUTTON1_DESC));
        SetWindowText(GetDlgItem(hwnd,IDD_WELC_BUTTON2),STR(STR_WELCOME_BUTTON2));
        SetWindowText(GetDlgItem(hwnd,IDD_WELC_BUTTON2_DESC),STR(STR_WELCOME_BUTTON2_DESC));
        SetWindowText(GetDlgItem(hwnd,IDD_WELC_BUTTON3),STR(STR_WELCOME_BUTTON3));
        SetWindowText(GetDlgItem(hwnd,IDD_WELC_BUTTON3_DESC),STR(STR_WELCOME_BUTTON3_DESC));
        SetWindowText(GetDlgItem(hwnd,IDD_WELC_CLOSE),STR(STR_WELCOME_CLOSE));
        // set focus to first button
        SetFocus(GetDlgItem(hwnd,IDD_WELC_BUTTON1));
        return TRUE;

    case WM_SETCURSOR:
        // 2 hyperlinks
        if ((LOWORD(lParam)==HTCLIENT) &&
            ((GetDlgCtrlID((HWND)wParam) == IDD_WELC_LINK1)||
             (GetDlgCtrlID((HWND)wParam) == IDD_WELC_LINK2)))
        {
            SetCursor(LoadCursor(nullptr, IDC_HAND));
            SetWindowLongPtr(hwnd, DWLP_MSGRESULT, TRUE);
            return true;
        }
        break;

    case WM_COMMAND:
        switch(wParam)
        {
            case IDD_WELC_CLOSE:
                EndDialog(hwnd,wParam);
                return TRUE;
            case IDCANCEL:
                EndDialog(hwnd,wParam);
                break;
            case IDD_WELC_BUTTON1:
                // download everything
                EndDialog(hwnd,wParam);
                Settings.flags&=~FLAG_AUTOUPDATE;
                #ifdef USE_TORRENT
                Updater->WelcomeDownloadAll();
                #endif
                return TRUE;
            case IDD_WELC_BUTTON2:
                // download network only
                EndDialog(hwnd,wParam);
                Settings.flags&=~FLAG_AUTOUPDATE;
                #ifdef USE_TORRENT
                Updater->WelcomeDownloadNetwork();
                #endif
                return TRUE;
            case IDD_WELC_BUTTON3:
                // download indexes only
                EndDialog(hwnd,wParam);
                Settings.flags&=~FLAG_AUTOUPDATE;
                #ifdef USE_TORRENT
                Updater->WelcomeDownloadIndexes();
                #endif
                return TRUE;
            case IDD_WELC_LINK1:
                System.run_command(L"open",WEB_HOMEPAGE,SW_SHOWNORMAL,0);
                break;
            default:
                break;
        }
        break;

    case WM_CTLCOLORSTATIC:
        {
            // modify the fonts for colours and bold and size etc
            Ctl1=GetDlgItem(hwnd,IDD_WELC_TITLE);
            Ctl2=GetDlgItem(hwnd,IDD_WELC_LINK1);
            //Ctl3=GetDlgItem(hwnd,IDD_WELC_LINK2);
            Ctl4=GetDlgItem(hwnd,IDD_WELC_SUBTITLE);
            HDC hdcStatic=(HDC)wParam;

            if((HWND)lParam==Ctl1)
            {
                HFONT hTitleFont = CreateFont(34,16,0,0,620,
                                             FALSE,FALSE,FALSE,
                                             ANSI_CHARSET,OUT_DEVICE_PRECIS,CLIP_MASK,
                                             ANTIALIASED_QUALITY,DEFAULT_PITCH,
                                             L"Tahoma");
                SetTextColor(hdcStatic, RGB(248,171,3));
                SelectObject(hdcStatic,hTitleFont);
            }
            else if((HWND)lParam==Ctl4)
            {
                HFONT hFont = CreateFont(24,0,0,0,620,
                                             FALSE,FALSE,FALSE,
                                             ANSI_CHARSET,OUT_DEVICE_PRECIS,CLIP_MASK,
                                             ANTIALIASED_QUALITY,DEFAULT_PITCH,
                                             L"Tahoma");
                SelectObject(hdcStatic,hFont);
            }
            else if((HWND)lParam==Ctl2)
            {
                HFONT hFont = CreateFont(18,0,0,0,750,
                                             FALSE,FALSE,FALSE,
                                             ANSI_CHARSET,OUT_DEVICE_PRECIS,CLIP_MASK,
                                             ANTIALIASED_QUALITY,DEFAULT_PITCH,
                                             L"MS Shell Dlg");
                SetTextColor(hdcStatic, RGB(0,0,255));
                SelectObject(hdcStatic,hFont);
            }

            SetBkMode(hdcStatic,TRANSPARENT);
            return (INT_PTR)g_hbrDlgBackground;
        }

    case WM_CTLCOLORDLG:
        return (INT_PTR)g_hbrDlgBackground;
    default:
        break;
    }
    return FALSE;
}


