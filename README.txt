DProffit

disksys is a virtual disk simulator

main features:
2 bitmaps, one for inodes in block 1 and one for data blocks in block 2
can save files
can view files in the same directory

expected function calls:
disksys.SetupFileSystem()
	Starts up disk system with a base directory

disksys.CreateFile(fileName, fileContent)
	Will make a file and store the content provided

code explination:
	when the file system is initialized, it makes a starting directory for all preseeding files to go into. From there, files can be added as pleased and what files are stored can be viewed. When a file or directory is required to be saved, the data is serialized and formatted into 1024 byte blocks. The system then uses bit maps to find free positions in the allocated data block area to save the data to. An array of the block locations is returned to the system. The system then creates an inode for the data blocks. An available spot is then found using the inode bitmap then the inode would be saved. If there are more data blocks left then another inode is made and linked to the previous inode. When a file is made, the location of the files inode is found using a current working directory global var. The directory is then opened to have a new directory entry struct added to its contents. The directory entry stores the name of the file and the location of that files first (or only) inode. When viewing the files in the directory, the directory is pulled up and all of the entries are displayed.
