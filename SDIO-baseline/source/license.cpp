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

#include "license.h"


extern HBRUSH g_hbrDlgBackground; // main.cpp

INT_PTR CALLBACK LicenseProcedure(HWND hwnd,UINT Message,WPARAM wParam,LPARAM lParam)
{
    WINDOWPOS *wpos;
    HWND hEditBox;
    RECT rect;
    LPCSTR s;
    size_t sz;

    switch(Message)
    {
        case WM_INITDIALOG:
            get_resource(IDR_LICENSE,(void **)&s,&sz);
            hEditBox=GetDlgItem(hwnd,IDC_EDIT1);
            SetWindowTextA(hEditBox,s);
            SendMessage(hEditBox,EM_SETREADONLY,1,0);
            // only show decline button on startup
            if(GetParent(hwnd))
            {
                ShowWindow(GetDlgItem(hwnd,IDCANCEL),SW_HIDE);
                SetFocus(GetDlgItem(hwnd,IDOK));
            }
            return TRUE;

        case WM_COMMAND:
            switch(LOWORD(wParam))
            {
                case IDOK:
                    Settings.license=2;
                    EndDialog(hwnd,IDOK);
                    return TRUE;

                case IDCANCEL:
                    if(!GetParent(hwnd))Settings.license=0;
                    EndDialog(hwnd,IDCANCEL);
                    return TRUE;

                default:
                    break;
            }
            break;

        case WM_WINDOWPOSCHANGED:
            wpos=(WINDOWPOS*)lParam;
            {
                int r=SystemParametersInfo(SPI_GETWORKAREA,0,&rect,0);
                if(r&&wpos->cy-rect.bottom>0)
                {
                    int sz1=rect.bottom-20-wpos->cy;
                    wpos->y=10;
                    wpos->cy=rect.bottom-20;
                    MoveWindow(hwnd,wpos->x,wpos->y,wpos->cx,wpos->cy,1);

                    GetRelativeCtrlRect(GetDlgItem(hwnd,IDC_EDIT1),&rect);
                    rect.bottom+=sz1;
                    MoveWindow(GetDlgItem(hwnd,IDC_EDIT1),rect.left,rect.top,rect.right,rect.bottom,1);

                    GetRelativeCtrlRect(GetDlgItem(hwnd,IDOK),&rect);
                    rect.top+=sz1;
                    MoveWindow(GetDlgItem(hwnd,IDOK),rect.left,rect.top,rect.right,rect.bottom,1);

                    GetRelativeCtrlRect(GetDlgItem(hwnd,IDCANCEL),&rect);
                    rect.top+=sz1;
                    MoveWindow(GetDlgItem(hwnd,IDCANCEL),rect.left,rect.top,rect.right,rect.bottom,1);
                }
            }
            return TRUE;

        case WM_CTLCOLORSTATIC:
            hEditBox=GetDlgItem(hwnd,IDC_EDIT1);
            if((HWND)lParam==hEditBox)
            {
                HDC hdcStatic=(HDC)wParam;
                SetTextColor(hdcStatic, GetSysColor(COLOR_WINDOWTEXT));
                SetBkColor(hdcStatic, GetSysColor(COLOR_WINDOW));
                return (LRESULT)GetStockObject(HOLLOW_BRUSH);
            }
            else
            {
                HDC hdcStatic=(HDC)wParam;
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
