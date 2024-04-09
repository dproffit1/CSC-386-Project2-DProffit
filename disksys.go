package disksys

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"strings"
	"time"
)

const inodeBitmapIndex = 0
const firstInodeBlock = 2
const InodesCount = 6 * 1024
const BlockSize = 1024
const dataBitmapIndex = 1
const firstDataBlock = 11

var Disk [6144][1024]byte // 6 Megs of data
var currentWorkingDirectory string

func SaveToDisk(data interface{}, isDirectory bool, createdTime, lastModified time.Time) (int, error) {
	var serializedData []byte
	var err error

	// Serialize data based on whether it represents a directory or not.
	if isDirectory {
		dirEntries, ok := data.([]DirectoryEntry)
		if !ok {
			return -1, fmt.Errorf("data is not of type []DirectoryEntry")
		}
		fmt.Println("saving Directory...")
		serializedData, err = SerializeDataDir(dirEntries)
	} else {
		fmt.Println("saving File...")
		serializedData, err = SerializeData(data)
	}
	if err != nil {
		fmt.Println("Error serializing data:", err)
		return -1, err
	}

	// Use AllocateAndSaveData to store the data and get the block indices.
	blockIndices, err := AllocateAndSaveData(serializedData)
	if err != nil {
		return -1, err
	}

	if err != nil {
		return -1, err
	}

	// Prepare variables for inode handling
	var previousInodeIndex int = -1
	var firstInodeIndex int = -1

	// Process the data blocks, creating inodes as needed
	for i := 0; i < len(blockIndices); i += 3 {
		// Prepare the directBlocks array for the current inode
		var directBlocks [3]int
		for j := 0; j < 3 && i+j < len(blockIndices); j++ {
			directBlocks[j] = blockIndices[i+j]
		}

		// Create an inode for the current chunk of blocks
		inode := Inode{
			IsValid:       true,
			IsDirectory:   isDirectory,
			DirectBlocks:  directBlocks,
			IndirectBlock: -1, // Initially no indirect block, might be updated later
			CreatedTime:   createdTime,
			LastModified:  lastModified,
			Size:          len(serializedData),
		}

		// Save the inode to disk and update linkage if necessary
		inodeIndex, err := AllocateAndSaveInode(inode)
		if err != nil {
			fmt.Println("Error during inode allocation and saving:", err)
			return -1, err
		}

		// Link this inode with the previous one, if it exists
		if previousInodeIndex != -1 {
			err = UpdateInodeIndirectBlock(previousInodeIndex, inodeIndex)
			if err != nil {
				fmt.Println("Error updating indirect block:", err)
				return -1, err
			}
		} else {
			firstInodeIndex = inodeIndex // First inode index to be returned
		}

		previousInodeIndex = inodeIndex // Update for the next iteration
	}

	// Return the index of the first inode used to save this data
	return firstInodeIndex, nil
}

func SetupFileSystem() error {
	name, err := EncodeFileName("start.str")
	if err != nil {
		return fmt.Errorf("invalid name %v", err)
	}

	StartDirect := DirectoryEntry{
		Name:       name,
		InodeIndex: 1,
	}

	// Create a slice containing only StartDirect
	dirEntries := []DirectoryEntry{StartDirect}

	_, err1 := SaveToDisk(dirEntries, true, time.Now(), time.Now())
	if err1 != nil {
		return fmt.Errorf("failed to create the root directory: %v", err)
	}

	currentWorkingDirectory = "/"

	return nil
}

func CreateFile(name, content string) error {
	// Save the file data to disk and get the first inode index of the saved data.
	createdTime := time.Now()
	firstInodeIndex, err := SaveToDisk(content, false, createdTime, createdTime)
	if err != nil {
		return fmt.Errorf("error saving file to disk: %v", err)
	}

	// Add the file's entry to the current directory.
	err = AddEntryToCurrentDirectory(name, firstInodeIndex)
	if err != nil {
		return fmt.Errorf("error adding file entry to current directory: %v", err)
	}

	return nil
}

func AddEntryToCurrentDirectory(name string, newInodeIndex int) error {
	currentWorkingDirectoryInodeIndex, err := FindInodeIndexByPath(currentWorkingDirectory)
	if err != nil {
		return fmt.Errorf("error finding current working directory inode: %v", err)
	}

	cwdInode, err := LoadInode(currentWorkingDirectoryInodeIndex)
	if err != nil {
		return fmt.Errorf("error loading current working directory inode: %v", err)
	}

	fmt.Print(cwdInode)

	var allEntries []DirectoryEntry

	// Deserialize directory entries from each block independently
	for _, blockIndex := range cwdInode.DirectBlocks {

		if blockIndex <= 0 {
			continue // Skip unused blocks
		}

		directoryEntries, err := DeserializeDataDir([]int{blockIndex})
		if err != nil {
			return fmt.Errorf("error deserializing data block at index %d: %v", blockIndex, err)
		}

		allEntries = append(allEntries, directoryEntries...)
	}

	// Create and append the new directory entry
	newEntry := DirectoryEntry{InodeIndex: newInodeIndex}
	fmt.Println(newEntry)
	copy(newEntry.Name[:], name)
	allEntries = append(allEntries, newEntry)

	// Serialize the updated directory entries
	updatedDirData, err := SerializeDataDir(allEntries)
	if err != nil {
		return fmt.Errorf("error serializing updated directory data: %v", err)
	}

	// Save the updated directory entries back to the disk
	// This step might need more logic to handle updating existing blocks or allocating new ones
	_, err = AllocateAndSaveData(updatedDirData)
	if err != nil {
		return fmt.Errorf("error saving updated directory data: %v", err)
	}

	return nil
}

// GetEntryFromCurrentDirectory retrieves the content of a file from the current directory.
func GetEntryFromCurrentDirectory(name string) ([]byte, error) {
	// Find the inode index associated with the current directory.
	currentDirectoryInodeIndex, err := FindInodeIndexByPath(currentWorkingDirectory)
	if err != nil {
		return nil, fmt.Errorf("error finding inode index for current directory: %v", err)
	}

	// Load the inode of the current directory.
	currentDirectoryInode, err := LoadInode(currentDirectoryInodeIndex)
	if err != nil {
		return nil, fmt.Errorf("error loading inode for current directory: %v", err)
	}

	// Deserialize directory entries from each block independently.
	var allEntries []DirectoryEntry
	for _, blockIndex := range currentDirectoryInode.DirectBlocks {
		if blockIndex <= 0 {
			continue // Skip unused blocks
		}

		directoryEntries, err := DeserializeDataDir([]int{blockIndex})
		if err != nil {
			return nil, fmt.Errorf("error deserializing data block at index %d: %v", blockIndex, err)
		}

		allEntries = append(allEntries, directoryEntries...)
	}

	fmt.Println(allEntries)

	// Find the entry with the matching name.
	var fileInodeIndex int
	for _, entry := range allEntries {
		if string(entry.Name[:]) == name {
			fileInodeIndex = entry.InodeIndex
			break
		}
	}

	// Load the inode of the file.
	fileInode, err := LoadInode(fileInodeIndex)
	if err != nil {
		return nil, fmt.Errorf("error loading inode for file '%s': %v", name, err)
	}

	// Retrieve the content of the file using the data block indices.
	content, err := LoadFile(fileInode.DirectBlocks[:])
	if err != nil {
		return nil, fmt.Errorf("error loading file content for '%s': %v", name, err)
	}

	return content, nil
}

func FindInodeIndexByPath(path string) (int, error) {
	// Start from the root directory inode.
	currentInodeIndex := 0 // Assuming the root directory's inode index is 0.

	// Special case for root directory.
	if path == "/" {
		return currentInodeIndex, nil
	}

	// Split the path into components.
	pathComponents := strings.Split(strings.Trim(path, "/"), "/")

	for _, component := range pathComponents {
		// Load the current directory inode.
		inode, err := LoadInode(currentInodeIndex)
		if err != nil {
			return -1, fmt.Errorf("error loading inode at index %d: %v", currentInodeIndex, err)
		}

		// Ensure the inode is a directory before trying to read its content.
		if !inode.IsDirectory {
			return -1, fmt.Errorf("path component '%s' is not a directory", component)
		}

		// Assume directory entries are stored directly in the inode's data blocks.
		found := false
		for _, blockIndex := range inode.DirectBlocks {
			if blockIndex == -1 {
				continue // Skip unused block references.
			}

			// Load and deserialize the directory entries from the block.
			directoryEntries, err := DeserializeDataDir([]int{blockIndex})
			if err != nil {
				return -1, fmt.Errorf("error deserializing data block at index %d: %v", blockIndex, err)
			}

			// Search for the path component among the directory entries.
			for _, entry := range directoryEntries {
				if string(entry.Name[:]) == component {
					// Move to the next component in the path.
					currentInodeIndex = entry.InodeIndex
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			return -1, fmt.Errorf("path component '%s' not found", component)
		}
	}

	// If we've successfully traversed the entire path, return the final inode index.
	return currentInodeIndex, nil
}

// UpdateInodeIndirectBlock updates the IndirectBlock of an inode with a given inodeIndex.
func UpdateInodeIndirectBlock(inodeIndex, newIndirectBlockIndex int) error {
	// Load the inode.
	inode, err := LoadInode(inodeIndex)
	if err != nil {
		return err
	}

	// Update the IndirectBlock.
	inode.IndirectBlock = newIndirectBlockIndex

	// Save the updated inode back to disk.
	return SaveInode(inodeIndex, inode)
}

type Inode struct {
	IsValid       bool      // True if this inode is an allocated file
	IsDirectory   bool      // True if this inode represents a directory
	DirectBlocks  [3]int    // Indices of the direct data blocks
	IndirectBlock int       // Index of the indirect data block
	CreatedTime   time.Time // Time when the inode was created
	LastModified  time.Time // Time when the inode was last modified
	Size          int       // Size of the file or directory in bytes
}

type DirectoryEntry struct { // seemed easy to kinda copy a struct like with the inode
	Name       [12]byte // 8 for name, 3 for extension, 1 for dot
	InodeIndex int      // Inode index
}

func EncodeFileName(fileName string) ([12]byte, error) {
	var encodedName [12]byte

	// Check if fileName exceeds 12 characters (8 for name + 1 for dot + 3 for extension)
	if len(fileName) > 12 {
		return encodedName, fmt.Errorf("file name exceeds the maximum length of 12 characters")
	}

	// Check for the format correctness (8.3)
	dotIndex := -1
	for i, ch := range fileName {
		if ch == '.' {
			dotIndex = i
			break
		}
	}

	// Ensure there's a dot and it's correctly placed
	if dotIndex == -1 || dotIndex > 8 || len(fileName)-dotIndex-1 > 3 {
		return encodedName, fmt.Errorf("file name does not conform to the 8.3 format")
	}

	return encodedName, nil
}

func AllocateAndSaveInode(inode Inode) (int, error) {
	// Allocate an inode index
	inodeIndex, err := AllocateInode()
	if err != nil {
		fmt.Println("Error allocating inode:", err)
		return -1, err
	}

	// Serialize and save the inode to the disk at the allocated index
	err = SaveInode(inodeIndex, inode)
	if err != nil {
		fmt.Println("Error saving inode to disk:", err)
		return -1, err
	}

	// Return the allocated inode index for further reference if needed
	return inodeIndex, nil
}

func AllocateAndSaveData(serializedData []byte) ([]int, error) {
	// Determine how many blocks are needed for the data.
	blocksNeeded := CalculateBlocksNeeded(len(serializedData))

	var blockIndices []int // Dynamically sized slice to hold the indices of allocated blocks.

	for i := 0; i < blocksNeeded; i++ {
		// Allocate a block.
		blockIndex, err := AllocateDataBlock()

		if err != nil {
			// On error, return what we have so far (which might be nothing).
			return blockIndices, err
		}

		// Append the index of the allocated block to our slice.
		blockIndices = append(blockIndices, blockIndex)

		// Calculate the portion of serializedData to write in this block.
		start := i * BlockSize
		end := start + BlockSize
		if end > len(serializedData) {
			end = len(serializedData)
		}

		// Save the chunk of data to the allocated block.
		err = SaveDataBlock(blockIndex, serializedData[start:end])
		if err != nil {
			return blockIndices, err
		}
	}

	// Return the slice of all used block indices.
	return blockIndices, nil
}

func CalculateBlocksNeeded(dataSize int) int {
	// Calculate the base number of blocks needed.
	blocksNeeded := dataSize / BlockSize

	// If there is any remainder, add an additional block to account for it.
	if dataSize%BlockSize > 0 {
		blocksNeeded += 1
	}

	return blocksNeeded
}

func AllocateInode() (int, error) {
	for i := 0; i < InodesCount; i++ {
		if !IsInodeBitSet(i) {
			SetInodeBit(i)
			return i + firstInodeBlock, nil // Adjusted to return correct block index
		}
	}
	return -1, fmt.Errorf("no free inodes available")
}

func AllocateDataBlock() (int, error) {
	// Starting search from the first data block to end of disk space
	for i := 0; i < len(Disk); i++ {
		if !IsDataBitSet(i) { // Adjusting check to new index
			SetDataBit(i) // Adjust for data bitmap indexing
			return i + firstDataBlock, nil
		}
	}
	return -1, fmt.Errorf("no free data blocks available")
}

// LoadFile loads the content of a file from given data block indices.
func LoadFile(dataBlockIndices []int) ([]byte, error) {
	var content []byte

	// Iterate through each data block index.
	for _, blockIndex := range dataBlockIndices {
		// Check if the data block index is valid.
		if blockIndex < firstDataBlock || blockIndex >= len(Disk) {
			return nil, fmt.Errorf("invalid data block index: %d", blockIndex)
		}

		// Read the content from the disk.
		blockContent, err := DeserializeDataFile([]int{blockIndex})
		if err != nil {
			return nil, fmt.Errorf("error deserializing data block at index %d: %v", blockIndex, err)
		}

		// Type assert blockContent to []byte and append it to content.
		content = append(content, blockContent.([]byte)...)
	}

	return content, nil
}

func SaveInode(inodeIndex int, inode Inode) error {
	inodeBytes, err := SerializeInode(inode) // turn inode to binary data
	if err != nil {
		return err
	}

	// Ensure inode data does not exceed the allocated space.
	if len(inodeBytes) > BlockSize {
		return fmt.Errorf("inode size exceeds allocated space")
	}

	copy(Disk[inodeIndex][:], inodeBytes)

	return nil
}

func SaveDataBlock(blockIndex int, data []byte) error {
	// Ensure blockIndex is within the valid range of data blocks
	if blockIndex < firstDataBlock || blockIndex >= len(Disk) {
		return fmt.Errorf("block index out of range")
	}

	// Check if data slice is well-formed: length should not exceed BlockSize
	if len(data) > BlockSize {
		return fmt.Errorf("data exceeds block size")
	}

	// Additional safety check: ensure the blockIndex points to a valid Disk block
	if blockIndex >= len(Disk) {
		return fmt.Errorf("blockIndex %d is out of the Disk array bounds", blockIndex)
	}

	// Perform the copy operation
	copy(Disk[blockIndex][:], data)

	return nil
}

func LoadInode(inodeIndex int) (Inode, error) {

	// Deserialize the inodeBytes back into an inode structure
	loadedInode, err := DeserializeInode(inodeIndex + firstInodeBlock)
	if err != nil {
		fmt.Println("Error deserializing inode from disk:", err)
	}

	return loadedInode, nil
}

func SerializeInode(inode Inode) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := gob.NewEncoder(&buffer)
	err := encoder.Encode(inode)
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func SerializeData(data interface{}) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := gob.NewEncoder(&buffer)
	if err := encoder.Encode(data); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func SerializeDataDir(entries []DirectoryEntry) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := gob.NewEncoder(&buffer)
	if err := encoder.Encode(entries); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func DeserializeInode(blockIndex int) (Inode, error) {
	var inode Inode

	data := Disk[blockIndex][:]
	buffer := bytes.NewBuffer(data)
	err := gob.NewDecoder(buffer).Decode(&inode)
	if err != nil {
		return Inode{}, err
	}
	return inode, nil
}

func DeserializeDataFile(blockIndices []int) (interface{}, error) {
	var dataBytes []byte

	// Assuming all data collectively fits within the allocated blocks.
	for _, blockIndex := range blockIndices {
		if blockIndex >= len(Disk) || blockIndex < 0 {
			return nil, fmt.Errorf("block index %d out of range", blockIndex)
		}
		dataBytes = append(dataBytes, Disk[blockIndex][:]...)
	}

	// Use a bytes.Buffer to read the data bytes into a gob.Decoder.
	decoder := gob.NewDecoder(bytes.NewReader(dataBytes))

	var data interface{}
	if err := decoder.Decode(&data); err != nil {
		return nil, fmt.Errorf("error deserializing data: %v", err)
	}

	return data, nil
}

func DeserializeDataDir(blockIndices []int) ([]DirectoryEntry, error) {
	var allEntries []DirectoryEntry

	// Iterate through each block index
	for _, blockIndex := range blockIndices {

		// Load the data from disk
		data := Disk[blockIndex][:] // Convert [1024]byte array to []byte slice

		// Decode the data into a slice of DirectoryEntry structs
		var directoryEntries []DirectoryEntry
		decoder := gob.NewDecoder(bytes.NewReader(data))
		if err := decoder.Decode(&directoryEntries); err != nil {
			return nil, fmt.Errorf("error deserializing data block at index %d: %v", blockIndex, err)
		}

		// Append the directory entries to the result
		allEntries = append(allEntries, directoryEntries...)
	}

	return allEntries, nil
}

func SetInodeBit(inodeIndex int) {
	byteIndex := inodeIndex / 8
	bitOffset := inodeIndex % 8
	Disk[inodeBitmapIndex][byteIndex] |= 1 << bitOffset
}

func ClearInodeBit(inodeIndex int) {
	byteIndex := inodeIndex / 8
	bitOffset := inodeIndex % 8
	Disk[inodeBitmapIndex][byteIndex] &^= 1 << bitOffset // found this on stack overflow, glad it works because idk how this works
}

func IsInodeBitSet(inodeIndex int) bool {
	byteIndex := inodeIndex / 8
	bitOffset := inodeIndex % 8
	return Disk[inodeBitmapIndex][byteIndex]&(1<<bitOffset) != 0
}

func SetDataBit(blockIndex int) {
	byteIndex := blockIndex / 8
	bitOffset := blockIndex % 8
	Disk[dataBitmapIndex][byteIndex] |= 1 << bitOffset
}

func ClearDataBit(blockIndex int) {
	byteIndex := blockIndex / 8
	bitOffset := blockIndex % 8
	Disk[dataBitmapIndex][byteIndex] &^= 1 << bitOffset // found this on stack overflow, glad it works because idk how this works
}

func IsDataBitSet(blockIndex int) bool {
	byteIndex := blockIndex / 8
	bitOffset := blockIndex % 8
	return (Disk[dataBitmapIndex][byteIndex] & (1 << bitOffset)) != 0
}
