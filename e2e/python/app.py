import cowsay

class Cow:
    def __init__(self, name):
        self.name = name

    def say_hello(self):
        cowsay.cow(f"Hello I'm {self.name} from rules_img! Moooh.")
