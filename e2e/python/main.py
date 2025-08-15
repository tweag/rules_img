#!/usr/bin/env python3

import platform
import sys

def main():
    print(f"Hello, world from {platform.system()} {platform.machine()}!")
    print(f"Python version: {sys.version.split()[0]}")

if __name__ == "__main__":
    main()
